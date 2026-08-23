package api

import (
	"fmt"
	"testing"
	"time"

	"NowhereDash/internal/models"
	"NowhereDash/internal/tunnel"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestUnifiedTrendDataUsesServiceHistory(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	database, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := database.AutoMigrate(&models.ServiceHistory{}); err != nil {
		t.Fatalf("migrate service history: %v", err)
	}

	recordTime := time.Now().Add(-2 * time.Minute).Truncate(time.Minute)
	record := models.ServiceHistory{
		EndpointID: 1,
		InstanceID: "portal-instance",
		RecordTime: recordTime,
		DeltaTCPIn: 10, DeltaTCPOut: 20, DeltaUDPIn: 30, DeltaUDPOut: 40,
		AvgPing: 5, AvgPool: 6, AvgTCPs: 7, AvgUDPs: 8,
		AvgSpeedIn: 9, AvgSpeedOut: 11,
	}
	if err := database.Create(&record).Error; err != nil {
		t.Fatalf("create service history: %v", err)
	}

	handler := &TunnelMetricsHandler{tunnelService: tunnel.NewService(database)}
	result, err := handler.getUnifiedTrendDataFromServiceHistory("portal-instance", 1)
	if err != nil {
		t.Fatalf("get unified trend: %v", err)
	}

	timestamps := result["traffic"].(map[string]interface{})["created_at"].([]int64)
	traffic := result["traffic"].(map[string]interface{})["avg_delay"].([]float64)
	ping := result["ping"].(map[string]interface{})["avg_delay"].([]float64)
	for index, timestamp := range timestamps {
		if timestamp != recordTime.UnixMilli() {
			continue
		}
		if traffic[index] != 100 || ping[index] != 5 {
			t.Fatalf("recorded metrics = traffic:%v ping:%v", traffic[index], ping[index])
		}
		return
	}
	t.Fatalf("record timestamp %d missing from trend", recordTime.UnixMilli())
}
