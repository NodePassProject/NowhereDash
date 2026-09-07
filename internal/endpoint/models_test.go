package endpoint

import (
	"encoding/json"
	"strings"
	"testing"

	"NowhereDash/internal/models"
)

func TestEndpointJSONOmitsControllerVersion(t *testing.T) {
	version := "2.0.1"
	item := models.Endpoint{
		ID:                    1,
		Name:                  "node",
		Ver:                   &version,
		SupportsSystemMonitor: true,
	}
	payload, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal endpoint: %v", err)
	}

	if strings.Contains(string(payload), version) || strings.Contains(string(payload), `"ver"`) {
		t.Fatalf("controller version leaked in endpoint JSON: %s", payload)
	}
	if !strings.Contains(string(payload), `"supportsSystemMonitor":true`) {
		t.Fatalf("capability flag missing from endpoint JSON: %s", payload)
	}
}
