package endpointtraffic

import (
	"path/filepath"
	"testing"
	"time"

	"NowhereDash/internal/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "traffic.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&models.Endpoint{}, &models.Tunnel{}, &models.EndpointTrafficCursor{}); err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := database.DB()
	t.Cleanup(func() { sqlDB.Close() })
	return database
}

func instant(value string) time.Time {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return t
}

func testEndpoint(t *testing.T, database *gorm.DB) models.Endpoint {
	t.Helper()
	ep := models.Endpoint{Name: "test", URL: "http://test", APIPath: "/api/v2", APIKey: "test"}
	if err := database.Create(&ep).Error; err != nil {
		t.Fatal(err)
	}
	return ep
}

func requireUsage(t *testing.T, database *gorm.DB, id, total, used int64) models.Endpoint {
	t.Helper()
	var ep models.Endpoint
	if err := database.First(&ep, id).Error; err != nil {
		t.Fatal(err)
	}
	if ep.TrafficTotal != total || ep.TrafficUsed != used {
		t.Fatalf("usage = total %d, period %d; want %d, %d", ep.TrafficTotal, ep.TrafficUsed, total, used)
	}
	return ep
}

func TestAccountingSurvivesDeletionResetAndReconnection(t *testing.T) {
	database := testDB(t)
	ep := testEndpoint(t, database)
	at := instant("2026-09-08T00:00:00Z")
	id := "portal-a"
	portal := models.Tunnel{Name: id, EndpointID: ep.ID, InstanceID: &id, Type: models.TunnelTypePortal,
		TCPRx: 100, TCPTx: 50, LastEventTime: models.NullTime{Time: at, Valid: true}}
	if err := database.Create(&portal).Error; err != nil {
		t.Fatal(err)
	}
	if err := SyncAll(database, at); err != nil {
		t.Fatal(err)
	}
	requireUsage(t, database, ep.ID, 150, 150)

	sample := models.EndpointTrafficCursor{EndpointID: ep.ID, InstanceID: id, TCPRx: 140, TCPTx: 80, ObservedAt: at.Add(time.Second)}
	for i := 0; i < 2; i++ {
		if err := Observe(database, sample, sample.ObservedAt); err != nil {
			t.Fatal(err)
		}
	}
	requireUsage(t, database, ep.ID, 220, 220)
	// An old database snapshot or out-of-order SSE event must not count twice.
	if err := SyncAll(database, at.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	stale := sample
	stale.ObservedAt, stale.TCPRx = at, 900
	if err := Observe(database, stale, at.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	requireUsage(t, database, ep.ID, 220, 220)

	// Counters reset independently; the first bytes after reset are included.
	sample.TCPRx, sample.TCPTx, sample.ObservedAt = 5, 100, at.Add(3*time.Second)
	if err := Observe(database, sample, sample.ObservedAt); err != nil {
		t.Fatal(err)
	}
	requireUsage(t, database, ep.ID, 245, 245)
	if err := database.Delete(&portal).Error; err != nil {
		t.Fatal(err)
	}
	if err := SyncAll(database, at.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}
	requireUsage(t, database, ep.ID, 245, 245)

	// A new service connection uses the persisted cursor, including deleted IDs.
	sample.TCPRx, sample.ObservedAt = 8, at.Add(5*time.Second)
	if err := Observe(database.Session(&gorm.Session{NewDB: true}), sample, sample.ObservedAt); err != nil {
		t.Fatal(err)
	}
	requireUsage(t, database, ep.ID, 248, 248)
}

func TestMonthlyRenewalIsIdempotentAndCatchesUpAfterDowntime(t *testing.T) {
	database := testDB(t)
	ep := testEndpoint(t, database)
	limit := int64(1024)
	plan := Plan{Currency: "CNY", MonthlyTrafficLimit: &limit, AutoResetTraffic: true,
		TrafficResetDay: 31, TrafficResetTime: "08:30", TrafficResetTimezone: "Asia/Singapore"}
	start := instant("2026-01-01T00:00:00Z")
	if err := UpdatePlan(database, ep.ID, plan, start); err != nil {
		t.Fatal(err)
	}
	sample := models.EndpointTrafficCursor{EndpointID: ep.ID, InstanceID: "portal", TCPRx: 500, ObservedAt: start}
	if err := Observe(database, sample, start); err != nil {
		t.Fatal(err)
	}
	before := instant("2026-01-31T00:29:59Z")
	if err := SyncAll(database, before); err != nil {
		t.Fatal(err)
	}
	requireUsage(t, database, ep.ID, 500, 500)
	due := instant("2026-01-31T00:30:00Z")
	if err := SyncAll(database, due); err != nil {
		t.Fatal(err)
	}
	current := requireUsage(t, database, ep.ID, 500, 0)
	if current.NextTrafficResetAt == nil || !current.NextTrafficResetAt.Equal(instant("2026-02-28T00:30:00Z")) {
		t.Fatalf("next reset = %v", current.NextTrafficResetAt)
	}
	sample.TCPRx, sample.ObservedAt = 520, due.Add(time.Second)
	if err := Observe(database, sample, sample.ObservedAt); err != nil {
		t.Fatal(err)
	}
	if err := SyncAll(database, sample.ObservedAt); err != nil {
		t.Fatal(err)
	}
	requireUsage(t, database, ep.ID, 520, 20)

	// Editing just the price must not push the renewal into the next month.
	price := 25.5
	plan.Price = &price
	if err := UpdatePlan(database, ep.ID, plan, sample.ObservedAt); err != nil {
		t.Fatal(err)
	}
	current = requireUsage(t, database, ep.ID, 520, 20)
	if !current.NextTrafficResetAt.Equal(instant("2026-02-28T00:30:00Z")) {
		t.Fatal("price edit changed schedule")
	}
	catchup := instant("2026-05-15T00:00:00Z")
	if err := SyncAll(database, catchup); err != nil {
		t.Fatal(err)
	}
	current = requireUsage(t, database, ep.ID, 520, 0)
	if !current.LastTrafficResetAt.Equal(instant("2026-04-30T00:30:00Z")) || !current.NextTrafficResetAt.Equal(instant("2026-05-31T00:30:00Z")) {
		t.Fatalf("catchup boundaries = %v, %v", current.LastTrafficResetAt, current.NextTrafficResetAt)
	}
	sample.TCPRx, sample.ObservedAt = 600, catchup.Add(time.Second)
	if err := Observe(database, sample, sample.ObservedAt); err != nil {
		t.Fatal(err)
	}
	if err := SyncAll(database, sample.ObservedAt); err != nil {
		t.Fatal(err)
	}
	requireUsage(t, database, ep.ID, 600, 80)
}

func TestNextResetCalendarBoundaries(t *testing.T) {
	for _, test := range []struct {
		name, after, want, zone, clock string
		day                            int
	}{
		{"leap year", "2028-02-01T00:00:00Z", "2028-02-29T00:00:00Z", "UTC", "00:00", 31},
		{"year rollover", "2026-12-31T00:00:00Z", "2027-01-31T00:00:00Z", "UTC", "00:00", 31},
		{"timezone", "2026-09-01T00:00:00Z", "2026-09-30T16:00:00Z", "Asia/Singapore", "00:00", 1},
		{"DST", "2026-03-01T00:00:00Z", "2026-03-15T04:00:00Z", "America/New_York", "00:00", 15},
	} {
		t.Run(test.name, func(t *testing.T) {
			next, err := NextReset(&models.Endpoint{TrafficResetDay: test.day, TrafficResetTime: test.clock, TrafficResetTimezone: test.zone}, instant(test.after))
			if err != nil || !next.Equal(instant(test.want)) {
				t.Fatalf("next = %v, %v; want %s", next, err, test.want)
			}
		})
	}
}

func TestPlanValidationAndDisablingRenewal(t *testing.T) {
	database := testDB(t)
	ep := testEndpoint(t, database)
	limit := int64(100)
	valid := Plan{Currency: "USD", MonthlyTrafficLimit: &limit, TrafficResetDay: 1, TrafficResetTime: "00:00", TrafficResetTimezone: "UTC", AutoResetTraffic: true}
	for _, mutate := range []func(*Plan){
		func(p *Plan) { p.TrafficResetDay = 32 }, func(p *Plan) { p.TrafficResetTime = "24:00" },
		func(p *Plan) { p.TrafficResetTimezone = "bad/timezone" }, func(p *Plan) { p.MonthlyTrafficLimit = nil },
		func(p *Plan) { p.Currency = "US" }, func(p *Plan) { n := -1.0; p.Price = &n },
	} {
		p := valid
		mutate(&p)
		if err := p.Validate(); err == nil {
			t.Fatalf("accepted invalid plan: %+v", p)
		}
	}
	now := instant("2026-09-08T00:00:00Z")
	if err := UpdatePlan(database, ep.ID, valid, now); err != nil {
		t.Fatal(err)
	}
	valid.AutoResetTraffic, valid.MonthlyTrafficLimit = false, nil
	if err := UpdatePlan(database, ep.ID, valid, now); err != nil {
		t.Fatal(err)
	}
	current := requireUsage(t, database, ep.ID, 0, 0)
	if current.NextTrafficResetAt != nil || current.MonthlyTrafficLimit != nil {
		t.Fatal("disabled plan retained its renewal or limit")
	}
}
