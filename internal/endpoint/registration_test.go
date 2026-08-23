package endpoint

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRegistrationTokenIsSingleUse(t *testing.T) {
	store := NewRegistrationTokenStore(10 * time.Minute)
	token, issued, err := store.Issue("edge-a")
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || issued.Name != "edge-a" {
		t.Fatalf("unexpected issued token claims: token=%q claims=%+v", token, issued)
	}

	var used RegistrationClaims
	if err := store.Use(token, func(claims RegistrationClaims) error {
		used = claims
		return nil
	}); err != nil {
		t.Fatalf("first token use failed: %v", err)
	}
	if used.Name != "edge-a" {
		t.Fatalf("used name = %q, want edge-a", used.Name)
	}
	if err := store.Use(token, func(RegistrationClaims) error { return nil }); !errors.Is(err, ErrInvalidRegistrationToken) {
		t.Fatalf("second token use error = %v, want ErrInvalidRegistrationToken", err)
	}
}

func TestRegistrationTokenFailureCanRetry(t *testing.T) {
	store := NewRegistrationTokenStore(10 * time.Minute)
	token, _, err := store.Issue("edge-b")
	if err != nil {
		t.Fatal(err)
	}

	wantErr := errors.New("endpoint name already exists")
	if err := store.Use(token, func(RegistrationClaims) error { return wantErr }); !errors.Is(err, wantErr) {
		t.Fatalf("callback error = %v, want %v", err, wantErr)
	}
	if err := store.Use(token, func(RegistrationClaims) error { return nil }); err != nil {
		t.Fatalf("retry after callback failure failed: %v", err)
	}
}

func TestRegistrationTokenExpires(t *testing.T) {
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	store := NewRegistrationTokenStore(10 * time.Minute)
	store.now = func() time.Time { return now }
	token, _, err := store.Issue("edge-c")
	if err != nil {
		t.Fatal(err)
	}

	now = now.Add(10 * time.Minute)
	if err := store.Use(token, func(RegistrationClaims) error { return nil }); !errors.Is(err, ErrInvalidRegistrationToken) {
		t.Fatalf("expired token error = %v, want ErrInvalidRegistrationToken", err)
	}
}

func TestBuildInstallerEndpointRequest(t *testing.T) {
	req, err := BuildInstallerEndpointRequest(InstallerRegistrationRequest{
		APIURL:   "https://[2001:db8::1]:8080/custom/v2",
		APIKey:   " secret-key ",
		Hostname: "[2001:db8::1]",
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.URL != "https://[2001:db8::1]:8080" || req.APIPath != "/custom/v2" || req.APIKey != "secret-key" {
		t.Fatalf("unexpected normalized request: %+v", req)
	}
}

func TestBuildInstallerEndpointRequestRejectsUnsafeValues(t *testing.T) {
	tests := []InstallerRegistrationRequest{
		{APIURL: "ftp://example.com/api/v2", APIKey: "key"},
		{APIURL: "https://user@example.com/api/v2", APIKey: "key"},
		{APIURL: "https://example.com/api/v2?debug=1", APIKey: "key"},
		{APIURL: "https://example.com/api//v2", APIKey: "key"},
		{APIURL: "https://example.com/api/v2", APIKey: "key\nother"},
		{APIURL: "https://example.com/api/v2", APIKey: "key", Hostname: "bad/host"},
	}
	for _, test := range tests {
		if _, err := BuildInstallerEndpointRequest(test); err == nil {
			t.Fatalf("BuildInstallerEndpointRequest(%+v) succeeded, want error", test)
		}
	}
}

func TestRegistrationTokenNameValidation(t *testing.T) {
	store := NewRegistrationTokenStore(time.Minute)
	for _, name := range []string{"", "bad\nname", strings.Repeat("x", 51)} {
		if _, _, err := store.Issue(name); err == nil {
			t.Fatalf("Issue(%q) succeeded, want error", name)
		}
	}
}
