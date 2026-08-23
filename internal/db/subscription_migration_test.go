package db

import (
	"NowhereDash/internal/models"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestDropLegacySubscriptionEnabledColumn(t *testing.T) {
	database, err := gorm.Open(sqlite.Open("file:legacy-subscription?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := database.Exec(`CREATE TABLE portal_subscriptions (id INTEGER PRIMARY KEY, enabled BOOLEAN NOT NULL)`).Error; err != nil {
		t.Fatalf("create legacy table: %v", err)
	}

	if err := dropLegacySubscriptionEnabledColumn(database); err != nil {
		t.Fatalf("drop legacy column: %v", err)
	}
	if database.Migrator().HasColumn(&models.PortalSubscription{}, "enabled") {
		t.Fatal("legacy enabled column still exists")
	}
}
