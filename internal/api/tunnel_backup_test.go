package api

import "testing"

func TestLegacyBackupPoolMigratesToMux(t *testing.T) {
	pool := int64(5)
	request := (BackupInstance{PoolSize: &pool}).portalRequest(1)
	if request.Mux != "1" {
		t.Fatalf("legacy pool was not migrated to mux: %q", request.Mux)
	}
}

func TestBackupMuxTakesPrecedenceOverLegacyPool(t *testing.T) {
	pool := int64(5)
	mux := "0"
	request := (BackupInstance{Mux: &mux, PoolSize: &pool}).portalRequest(1)
	if request.Mux != "0" {
		t.Fatalf("explicit mux did not take precedence: %q", request.Mux)
	}
}
