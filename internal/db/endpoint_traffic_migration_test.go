package db

import (
	"path/filepath"
	"testing"
	"time"

	"NowhereDash/internal/endpointtraffic"
	"NowhereDash/internal/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestEndpointTrafficMigrationPreservesExistingUsage(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "legacy.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`CREATE TABLE endpoints (id INTEGER PRIMARY KEY, name TEXT NOT NULL, url TEXT NOT NULL, api_path TEXT NOT NULL, api_key TEXT NOT NULL)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`INSERT INTO endpoints (id, name, url, api_path, api_key) VALUES (1, 'legacy', 'http://legacy', '/api/v2', 'test')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(database); err != nil {
		t.Fatal(err)
	}
	instanceID := "legacy-portal"
	portal := models.Tunnel{Name: "legacy", EndpointID: 1, InstanceID: &instanceID, Type: models.TunnelTypePortal, TCPRx: 100, TCPTx: 50}
	if err := database.Create(&portal).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := endpointtraffic.SyncAll(database, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	var ep models.Endpoint
	if err := database.First(&ep, 1).Error; err != nil {
		t.Fatal(err)
	}
	if ep.TrafficTotal != 150 || ep.TrafficUsed != 150 || ep.AutoResetTraffic {
		t.Fatalf("migrated endpoint: %+v", ep)
	}
	if err := database.Delete(&portal).Error; err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(database); err != nil {
		t.Fatal(err)
	}
	if err := endpointtraffic.SyncAll(database, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := database.First(&ep, 1).Error; err != nil {
		t.Fatal(err)
	}
	if ep.TrafficTotal != 150 {
		t.Fatalf("usage after restart/deletion: %d", ep.TrafficTotal)
	}
}
