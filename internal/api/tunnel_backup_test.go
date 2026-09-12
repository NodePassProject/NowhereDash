package api

import (
	"NowhereDash/internal/nowhere"
	"encoding/json"
	"testing"
)

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

func TestBackupV4PreservesEndpointAndMorph(t *testing.T) {
	portal := nowhere.ParseTunnelURL("portal://key@*/tcp4:2006/udp6:2017?morph=1")
	backup := tunnelToBackupInstance(*portal)
	data, err := json.Marshal(backup)
	if err != nil {
		t.Fatal(err)
	}
	var decoded BackupInstance
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	req := decoded.portalRequest(1)
	if req.TCPPort == nil || *req.TCPPort != "2006" || req.UDPPort == nil || *req.UDPPort != "2017" || req.TCPFamily != "4" || req.UDPFamily != "6" || req.Morph == nil || *req.Morph != "1" {
		t.Fatalf("v2 configuration lost in backup: %+v", req)
	}
}
