package db

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestMigrateLegacyTunnelPoolToMux(t *testing.T) {
	database, err := gorm.Open(sqlite.Open("file:legacy-tunnel-pool?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := database.Exec(`CREATE TABLE tunnels (id INTEGER PRIMARY KEY, pool_size INTEGER, mux TEXT)`).Error; err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	if err := database.Exec(`INSERT INTO tunnels (id, pool_size, mux) VALUES (1, 5, NULL), (2, 0, NULL), (3, 5, '0')`).Error; err != nil {
		t.Fatalf("seed legacy tunnels: %v", err)
	}

	if err := migrateLegacyTunnelPoolToMux(database); err != nil {
		t.Fatalf("migrate legacy pool: %v", err)
	}

	var rows []struct {
		ID  int64
		Mux string
	}
	if err := database.Table("tunnels").Select("id, mux").Order("id").Scan(&rows).Error; err != nil {
		t.Fatalf("read migrated tunnels: %v", err)
	}
	want := []string{"1", "0", "0"}
	for index, row := range rows {
		if row.Mux != want[index] {
			t.Fatalf("tunnel %d mux = %q, want %q", row.ID, row.Mux, want[index])
		}
	}
}
