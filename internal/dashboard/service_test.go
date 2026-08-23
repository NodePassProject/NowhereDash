package dashboard

import (
	"fmt"
	"testing"

	"NowhereDash/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestGetStatsUsesPortalTypeColumn(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	database, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := database.AutoMigrate(
		&models.Endpoint{},
		&models.Tunnel{},
		&models.TunnelOperationLog{},
	); err != nil {
		t.Fatalf("migrate dashboard tables: %v", err)
	}

	endpoint := models.Endpoint{
		Name:    "openctrl-a",
		URL:     "https://openctrl.example.com",
		APIPath: "/api/v2",
		APIKey:  "test-key",
		Status:  models.EndpointStatusOnline,
	}
	if err := database.Create(&endpoint).Error; err != nil {
		t.Fatalf("create endpoint: %v", err)
	}

	tunnel := models.Tunnel{
		Name:        "portal-a",
		EndpointID:  endpoint.ID,
		Type:        models.TunnelTypePortal,
		Status:      models.TunnelStatusRunning,
		ListenHost:  "0.0.0.0",
		ListenPort:  "443",
		TLSMode:     models.TLS1,
		LogLevel:    models.LogLevelInfo,
		CommandLine: "portal://shared@0.0.0.0:443?tls=1",
		TCPRx:       10,
		TCPTx:       20,
	}
	if err := database.Create(&tunnel).Error; err != nil {
		t.Fatalf("create portal: %v", err)
	}

	stats, err := NewService(database).GetStats(TimeRangeAllTime)
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.TunnelTypes.Portal != 1 || stats.TunnelTypes.Total != 1 {
		t.Fatalf("tunnel types = %+v, want one portal", stats.TunnelTypes)
	}
	if len(stats.TopTunnels) != 1 || stats.TopTunnels[0].Type != string(models.TunnelTypePortal) {
		t.Fatalf("top tunnels = %+v, want portal type", stats.TopTunnels)
	}
}
