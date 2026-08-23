package endpoint

import (
	"NowhereDash/internal/models"
	"NowhereDash/internal/nowhere"
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestCreateEndpointDefaultsAPIPath(t *testing.T) {
	db := newEndpointTestDB(t)
	nowhere.GetCache().Clear()

	created, err := NewService(db).CreateEndpoint(CreateEndpointRequest{
		Name:   "master-default-path",
		URL:    "http://master-default-path.example.com/",
		APIKey: "test-key",
	})
	if err != nil {
		t.Fatalf("CreateEndpoint returned error: %v", err)
	}
	if created.APIPath != nowhere.DefaultAPIPath {
		t.Fatalf("APIPath = %q, want %q", created.APIPath, nowhere.DefaultAPIPath)
	}

	baseURL, apiKey, exists := nowhere.GetCache().Get(fmt.Sprintf("%d", created.ID))
	if !exists || baseURL != "http://master-default-path.example.com/api/v2" || apiKey != "test-key" {
		t.Fatalf("cache = (%q, %q, %t), want normalized OpenCtrl endpoint", baseURL, apiKey, exists)
	}
}

func TestUpdateEndpointAPIPathRefreshesCache(t *testing.T) {
	for _, action := range []string{"update", "updateConfig"} {
		t.Run(action, func(t *testing.T) {
			db := newEndpointTestDB(t)
			nowhere.GetCache().Clear()
			service := NewService(db)
			created, err := service.CreateEndpoint(CreateEndpointRequest{
				Name:    "master-" + action,
				URL:     "http://master-" + action + ".example.com",
				APIPath: "/api/v2",
				APIKey:  "test-key",
			})
			if err != nil {
				t.Fatalf("CreateEndpoint returned error: %v", err)
			}

			updated, err := service.UpdateEndpoint(UpdateEndpointRequest{
				ID:      created.ID,
				Action:  action,
				APIPath: "custom/v3/",
			})
			if err != nil {
				t.Fatalf("UpdateEndpoint returned error: %v", err)
			}
			if updated.APIPath != "/custom/v3" {
				t.Fatalf("APIPath = %q, want normalized path", updated.APIPath)
			}

			baseURL, apiKey, exists := nowhere.GetCache().Get(fmt.Sprintf("%d", created.ID))
			if !exists || baseURL != "http://master-"+action+".example.com/custom/v3" || apiKey != "test-key" {
				t.Fatalf("cache = (%q, %q, %t), want updated API path", baseURL, apiKey, exists)
			}
		})
	}
}

func TestDeleteEndpointSkipsMissingOptionalCleanupTables(t *testing.T) {
	db := newEndpointTestDB(t)
	endpointID := createEndpointForDeleteTest(t, db, "master-a")

	for _, tableName := range optionalEndpointCleanupTables {
		if db.Migrator().HasTable(tableName) {
			t.Fatalf("optional table %s should not exist in this test", tableName)
		}
	}

	if err := NewService(db).DeleteEndpoint(endpointID); err != nil {
		t.Fatalf("DeleteEndpoint returned error with missing optional tables: %v", err)
	}

	var count int64
	if err := db.Model(&models.Endpoint{}).Where("id = ?", endpointID).Count(&count).Error; err != nil {
		t.Fatalf("count endpoint: %v", err)
	}
	if count != 0 {
		t.Fatalf("endpoint was not deleted, count=%d", count)
	}
}

func TestDeleteEndpointCleansExistingOptionalTables(t *testing.T) {
	db := newEndpointTestDB(t)
	endpointID := createEndpointForDeleteTest(t, db, "master-b")
	otherEndpointID := createEndpointForDeleteTest(t, db, "master-c")

	for _, tableName := range optionalEndpointCleanupTables {
		if err := db.Exec("CREATE TABLE " + tableName + " (id INTEGER PRIMARY KEY AUTOINCREMENT, endpoint_id INTEGER NOT NULL)").Error; err != nil {
			t.Fatalf("create optional table %s: %v", tableName, err)
		}
		if err := db.Exec("INSERT INTO "+tableName+" (endpoint_id) VALUES (?), (?)", endpointID, otherEndpointID).Error; err != nil {
			t.Fatalf("insert optional table %s: %v", tableName, err)
		}
	}

	if err := NewService(db).DeleteEndpoint(endpointID); err != nil {
		t.Fatalf("DeleteEndpoint returned error: %v", err)
	}

	for _, tableName := range optionalEndpointCleanupTables {
		var deletedCount int64
		if err := db.Table(tableName).Where("endpoint_id = ?", endpointID).Count(&deletedCount).Error; err != nil {
			t.Fatalf("count deleted rows in %s: %v", tableName, err)
		}
		if deletedCount != 0 {
			t.Fatalf("expected no rows for deleted endpoint in %s, got %d", tableName, deletedCount)
		}

		var keptCount int64
		if err := db.Table(tableName).Where("endpoint_id = ?", otherEndpointID).Count(&keptCount).Error; err != nil {
			t.Fatalf("count kept rows in %s: %v", tableName, err)
		}
		if keptCount != 1 {
			t.Fatalf("expected other endpoint row to remain in %s, got %d", tableName, keptCount)
		}
	}
}

func newEndpointTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	if err := db.AutoMigrate(
		&models.Endpoint{},
		&models.Tunnel{},
		&models.TunnelGroup{},
		&models.TunnelOperationLog{},
		&models.TrafficHourlySummary{},
		&models.ServiceHistory{},
	); err != nil {
		t.Fatalf("migrate endpoint test schema: %v", err)
	}

	return db
}

func createEndpointForDeleteTest(t *testing.T, db *gorm.DB, name string) int64 {
	t.Helper()

	endpoint := models.Endpoint{
		Name:    name,
		URL:     "http://" + name + ".example.com",
		APIPath: "/api",
		APIKey:  "test-key",
		Status:  models.EndpointStatusOffline,
	}
	if err := db.Create(&endpoint).Error; err != nil {
		t.Fatalf("create endpoint %s: %v", name, err)
	}
	return endpoint.ID
}
