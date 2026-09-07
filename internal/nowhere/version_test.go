package nowhere

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestVersionAtLeast(t *testing.T) {
	tests := []struct {
		actual   string
		required string
		want     bool
	}{
		{actual: "1.6.0", required: "1.6.0", want: true},
		{actual: "v1.10.0", required: "1.6.0", want: true},
		{actual: "1.5.9", required: "1.6.0", want: false},
		{actual: "1.10.0-beta1", required: "1.10.0", want: true},
	}

	for _, test := range tests {
		if got := VersionAtLeast(test.actual, test.required); got != test.want {
			t.Fatalf("VersionAtLeast(%q, %q) = %v, want %v", test.actual, test.required, got, test.want)
		}
	}
}

func TestEndpointInfoResultJSONOmitsControllerVersion(t *testing.T) {
	info := EndpointInfoResult{OS: "linux", Arch: "amd64", Ver: "2.0.1", CPU: 4}
	payload, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("marshal endpoint info: %v", err)
	}

	if strings.Contains(string(payload), "2.0.1") || strings.Contains(string(payload), `"ver"`) {
		t.Fatalf("controller version leaked in endpoint info JSON: %s", payload)
	}
	if !strings.Contains(string(payload), `"os":"linux"`) {
		t.Fatalf("public endpoint info fields missing from JSON: %s", payload)
	}
}
