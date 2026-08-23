package dashboard

import (
	"fmt"
	"testing"
	"time"

	"NowhereDash/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newTrafficTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&models.ServiceHistory{},
		&models.TrafficHourlySummary{},
		&models.DashboardTrafficSummary{},
	); err != nil {
		t.Fatalf("migrate traffic tables: %v", err)
	}
	if err := db.Exec(`
		CREATE UNIQUE INDEX uniq_traffic_hourly_summary_hour_endpoint_instance
		ON traffic_hourly_summary(hour_time, endpoint_id, instance_id)
	`).Error; err != nil {
		t.Fatalf("create hourly unique index: %v", err)
	}

	return db
}

func TestAggregateTrafficDataForHourNormalizesUTCAndKeepsInt64(t *testing.T) {
	db := newTrafficTestDB(t)
	service := NewTrafficService(db)
	local := time.FixedZone("UTC+8", 8*60*60)
	hourStart := time.Date(2026, 7, 12, 13, 0, 0, 0, local)

	history := []models.ServiceHistory{
		{
			EndpointID: 1, InstanceID: "a", RecordTime: hourStart.Add(17 * time.Minute).UTC(),
			DeltaTCPIn: 10, DeltaTCPOut: 2_500_000_000,
		},
		{
			EndpointID: 1, InstanceID: "a", RecordTime: hourStart.Add(59 * time.Minute).UTC(),
			DeltaTCPIn: 20, DeltaTCPOut: 2_800_000_000,
		},
		{
			EndpointID: 2, InstanceID: "b", RecordTime: hourStart.Add(59 * time.Minute).UTC(),
			DeltaTCPIn: 30, DeltaTCPOut: 1_200_000_000,
		},
		{
			// Duplicate latest timestamps are possible when an SSE event is
			// dispatched more than once. The higher ID must win deterministically.
			EndpointID: 1, InstanceID: "a", RecordTime: hourStart.Add(59 * time.Minute).UTC(),
			DeltaTCPIn: 25, DeltaTCPOut: 2_900_000_000,
		},
	}
	if err := db.Create(&history).Error; err != nil {
		t.Fatalf("insert service history: %v", err)
	}

	if err := service.AggregateTrafficDataForHour(hourStart); err != nil {
		t.Fatalf("aggregate hour: %v", err)
	}

	var hourlyCount int64
	if err := db.Model(&models.TrafficHourlySummary{}).Count(&hourlyCount).Error; err != nil {
		t.Fatalf("count hourly rows: %v", err)
	}
	if hourlyCount != 2 {
		t.Fatalf("hourly rows = %d, want 2", hourlyCount)
	}

	var summary models.DashboardTrafficSummary
	if err := db.First(&summary).Error; err != nil {
		t.Fatalf("load dashboard summary: %v", err)
	}
	if !summary.HourTime.Equal(hourStart.UTC()) {
		t.Fatalf("hour time = %s, want %s", summary.HourTime, hourStart.UTC())
	}
	if summary.TCPTxTotal != 4_100_000_000 {
		t.Fatalf("tcp tx total = %d, want 4100000000", summary.TCPTxTotal)
	}
	if summary.InstanceCount != 2 {
		t.Fatalf("instance count = %d, want 2", summary.InstanceCount)
	}

	var instanceA models.TrafficHourlySummary
	if err := db.Where("instance_id = ?", "a").First(&instanceA).Error; err != nil {
		t.Fatalf("load instance a summary: %v", err)
	}
	if instanceA.TCPTxIncrement != 400_000_000 {
		t.Fatalf("tcp tx increment = %d, want 400000000", instanceA.TCPTxIncrement)
	}
}

func TestAggregateTrafficDataForHourRemovesEmptyDashboardRow(t *testing.T) {
	db := newTrafficTestDB(t)
	service := NewTrafficService(db)
	hourStart := time.Date(2026, 7, 12, 5, 0, 0, 0, time.UTC)

	if err := db.Create(&models.DashboardTrafficSummary{
		HourTime: hourStart,
	}).Error; err != nil {
		t.Fatalf("insert stale dashboard row: %v", err)
	}
	if err := service.AggregateTrafficDataForHour(hourStart); err != nil {
		t.Fatalf("aggregate empty hour: %v", err)
	}

	var count int64
	if err := db.Model(&models.DashboardTrafficSummary{}).Count(&count).Error; err != nil {
		t.Fatalf("count dashboard rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("dashboard rows = %d, want 0", count)
	}
}

func TestCleanInvalidDashboardTrafficKeepsRealZeroTrafficRows(t *testing.T) {
	db := newTrafficTestDB(t)
	service := NewTrafficService(db)
	baseHour := time.Date(2026, 7, 10, 5, 0, 0, 0, time.UTC)

	rows := []models.DashboardTrafficSummary{
		{HourTime: baseHour, InstanceCount: 0},
		{HourTime: baseHour.Add(time.Hour), InstanceCount: 2},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("insert dashboard rows: %v", err)
	}
	if err := service.cleanInvalidDashboardTraffic(); err != nil {
		t.Fatalf("clean invalid dashboard rows: %v", err)
	}

	var remaining []models.DashboardTrafficSummary
	if err := db.Find(&remaining).Error; err != nil {
		t.Fatalf("load dashboard rows: %v", err)
	}
	if len(remaining) != 1 || remaining[0].InstanceCount != 2 {
		t.Fatalf("remaining dashboard rows = %+v, want one real zero-traffic row", remaining)
	}
}

func TestTodayAndWeeklyStatsUsePositiveHourlyIncrements(t *testing.T) {
	db := newTrafficTestDB(t)
	now := time.Now()
	todayLocal := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	rows := []models.TrafficHourlySummary{
		{
			HourTime: todayLocal.Add(time.Hour).UTC(), EndpointID: 1, InstanceID: "a",
			TCPRxIncrement: 100, TCPTxIncrement: 200, UDPRxIncrement: 300, UDPTxIncrement: 400,
		},
		{
			HourTime: todayLocal.Add(2 * time.Hour).UTC(), EndpointID: 2, InstanceID: "b",
			TCPRxIncrement: -50, TCPTxIncrement: 20, UDPRxIncrement: 30, UDPTxIncrement: 40,
		},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("insert hourly summaries: %v", err)
	}

	today, err := NewTrafficService(db).GetTodayTrafficIncrement()
	if err != nil {
		t.Fatalf("get today traffic: %v", err)
	}
	if today.TCPRx != 100 || today.TCPTx != 220 || today.UDPRx != 330 || today.UDPTx != 440 {
		t.Fatalf("unexpected today traffic: %+v", today)
	}

	weekly, err := NewService(db).GetWeeklyStats()
	if err != nil {
		t.Fatalf("get weekly traffic: %v", err)
	}
	var currentDay WeeklyStatsItem
	for _, item := range weekly {
		if item.Date == todayLocal.Format("2006-01-02") {
			currentDay = item
			break
		}
	}
	if currentDay.TCPIn != 100 || currentDay.TCPOut != 220 || currentDay.UDPIn != 330 || currentDay.UDPOut != 440 {
		t.Fatalf("unexpected weekly day traffic: %+v", currentDay)
	}
}
