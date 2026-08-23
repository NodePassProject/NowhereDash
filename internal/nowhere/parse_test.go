package nowhere

import (
	"NowhereDash/internal/models"
	"encoding/json"
	"net/url"
	"reflect"
	"testing"
)

func TestInstanceResultDecodesCommandAndConfigURLAliases(t *testing.T) {
	tests := []struct {
		name       string
		payload    string
		wantURL    string
		wantConfig string
	}{
		{
			name:       "canonical fields win",
			payload:    `{"url":"portal://canonical@:1001","commandURL":"portal://alias@:1002","config":"portal://canonical@:1001?rate=0","configURL":"portal://alias@:1002?rate=1"}`,
			wantURL:    "portal://canonical@:1001",
			wantConfig: "portal://canonical@:1001?rate=0",
		},
		{
			name:       "upper URL aliases",
			payload:    `{"commandURL":"portal://upper@:2001","configURL":"portal://upper@:2001?rate=0"}`,
			wantURL:    "portal://upper@:2001",
			wantConfig: "portal://upper@:2001?rate=0",
		},
		{
			name:       "lower URL aliases",
			payload:    `{"commandUrl":"portal://lower@:3001","configUrl":"portal://lower@:3001?rate=0"}`,
			wantURL:    "portal://lower@:3001",
			wantConfig: "portal://lower@:3001?rate=0",
		},
		{
			name:       "snake URL aliases",
			payload:    `{"command_url":"portal://snake@:4001","config_url":"portal://snake@:4001?rate=0"}`,
			wantURL:    "portal://snake@:4001",
			wantConfig: "portal://snake@:4001?rate=0",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var instance InstanceResult
			if err := json.Unmarshal([]byte(test.payload), &instance); err != nil {
				t.Fatalf("decode instance: %v", err)
			}
			if instance.URL != test.wantURL {
				t.Fatalf("URL = %q, want %q", instance.URL, test.wantURL)
			}
			if instance.Config == nil || *instance.Config != test.wantConfig {
				t.Fatalf("Config = %#v, want %q", instance.Config, test.wantConfig)
			}
		})
	}
}

func TestParseInstanceTunnelPrefersExpandedConfigURL(t *testing.T) {
	commandURL := "portal://runtime@:2077?net=tcp"
	configURL := "portal://:2077?net=tcp&tls=1&alpn=now%2F1&rate=0&etar=0&dial=auto&socks=none&next=none"
	parsed := ParseInstanceTunnel(InstanceResult{URL: commandURL, Config: &configURL})

	if parsed.CommandLine != commandURL || parsed.ConfigLine == nil || *parsed.ConfigLine != configURL {
		t.Fatalf("URL fields were not preserved: command=%q config=%#v", parsed.CommandLine, parsed.ConfigLine)
	}
	if parsed.SharedKey == nil || *parsed.SharedKey != "runtime" || parsed.Network == nil || *parsed.Network != "tcp" {
		t.Fatalf("runtime config did not win: key=%#v network=%#v", parsed.SharedKey, parsed.Network)
	}
	if parsed.ALPN == nil || *parsed.ALPN != "now/1" || parsed.Rate == nil || *parsed.Rate != 0 ||
		parsed.Etar == nil || *parsed.Etar != 0 || parsed.Dial == nil || *parsed.Dial != "auto" ||
		parsed.Socks == nil || *parsed.Socks != "none" || parsed.TLSMode != models.TLS1 ||
		parsed.LogLevel != models.LogLevelInfo {
		t.Fatalf("runtime defaults were not parsed: %+v", parsed)
	}
}

func TestEffectivePortalURLFallsBackFromInvalidConfig(t *testing.T) {
	commandURL := "portal://secret@:2077"
	invalidConfig := "runtime config unavailable"
	if got := PortalConfigURL(&invalidConfig); got != "" {
		t.Fatalf("invalid config URL was exposed as %q", got)
	}
	if got := EffectivePortalURL(commandURL, &invalidConfig); got != commandURL {
		t.Fatalf("effective URL = %q, want command URL %q", got, commandURL)
	}
}

func TestEffectivePortalURLRejectsStaleConfig(t *testing.T) {
	commandURL := "portal://secret@:2077?net=udp"
	staleConfig := "portal://:2077?net=tcp&rate=0"
	if got := MatchingPortalConfigURL(commandURL, &staleConfig); got != "" {
		t.Fatalf("stale config URL was accepted: %q", got)
	}
	parsed := ParseInstanceTunnel(InstanceResult{URL: commandURL, Config: &staleConfig})
	if parsed.Network == nil || *parsed.Network != "udp" {
		t.Fatalf("stale config overwrote command network: %#v", parsed.Network)
	}
	if parsed.ConfigLine == nil || *parsed.ConfigLine != "" {
		t.Fatalf("stale config must request stored config clearing: %#v", parsed.ConfigLine)
	}
}

func TestTunnelConfigJSONUsesDetailsFieldNames(t *testing.T) {
	payload, err := json.Marshal(ParseTunnelConfig("portal://secret@:2077"))
	if err != nil {
		t.Fatalf("marshal details config: %v", err)
	}
	var fields map[string]interface{}
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("decode details config: %v", err)
	}
	for _, key := range []string{"listenHost", "listenPort", "sharedKey", "tlsMode", "poolSize", "logLevel"} {
		if _, ok := fields[key]; !ok {
			t.Fatalf("camelCase field %q missing from %s", key, payload)
		}
	}
	if _, exists := fields["ListenHost"]; exists {
		t.Fatalf("unexpected exported Go field name in %s", payload)
	}
}

func TestParseInstanceTunnelKeepsRequiredCommandValues(t *testing.T) {
	commandURL := "portal://secret@:2077?net=mix&tls=2&crt=%2Fcert.pem&key=%2Fkey.pem&alpn=now%2F1&rate=0&etar=0&dial=auto&socks=user%3Apass%40127.0.0.1%3A1080&next=none&log=debug"
	configURL := "portal://:2077?net=mix&tls=2&alpn=now%2F1&rate=0&etar=0&dial=auto&socks=127.0.0.1%3A1080&next=none"
	parsed := ParseInstanceTunnel(InstanceResult{URL: commandURL, Config: &configURL})

	if parsed.ConfigLine == nil || *parsed.ConfigLine != configURL {
		t.Fatalf("credential-free runtime config was not accepted: %#v", parsed.ConfigLine)
	}
	if parsed.ListenPort != "2077" {
		t.Fatalf("missing runtime port did not fall back: %q", parsed.ListenPort)
	}
	if parsed.SharedKey == nil || *parsed.SharedKey != "secret" {
		t.Fatalf("missing runtime shared key did not fall back: %#v", parsed.SharedKey)
	}
	if parsed.CertPath == nil || *parsed.CertPath != "/cert.pem" || parsed.KeyPath == nil || *parsed.KeyPath != "/key.pem" {
		t.Fatalf("missing TLS paths did not fall back: crt=%#v key=%#v", parsed.CertPath, parsed.KeyPath)
	}
	if parsed.Socks == nil || *parsed.Socks != "user:pass@127.0.0.1:1080" || parsed.LogLevel != models.LogLevelDebug {
		t.Fatalf("redacted runtime values did not fall back: socks=%#v log=%q", parsed.Socks, parsed.LogLevel)
	}
}

func TestPortalURLRoundTrip(t *testing.T) {
	raw := "portal://secret@0.0.0.0:2077?net=tcp&tls=2&crt=%2Fcert.pem&key=%2Fkey.pem&alpn=now%2Fprivate&rate=100&etar=200&dial=auto&socks=none&next=origin%40relay.example%3A2077&up=tcp&down=tcp&pool=5&sni=relay.example&pin=none&log=warn"
	parsed := ParseTunnelURL(raw)
	if parsed.Type != models.TunnelTypePortal {
		t.Fatalf("expected portal type, got %q", parsed.Type)
	}
	if parsed.SharedKey == nil || *parsed.SharedKey != "secret" {
		t.Fatalf("shared key was not parsed")
	}
	if parsed.ListenHost != "0.0.0.0" || parsed.ListenPort != "2077" {
		t.Fatalf("unexpected listener %s:%s", parsed.ListenHost, parsed.ListenPort)
	}
	if err := ValidatePortalTunnel(*parsed); err != nil {
		t.Fatalf("valid Portal URL rejected: %v", err)
	}
	rebuilt := ParseTunnelURL(BuildTunnelURLs(*parsed))
	if rebuilt.Next == nil || *rebuilt.Next != "origin@relay.example:2077" {
		t.Fatalf("next was not preserved: %#v", rebuilt.Next)
	}
	if rebuilt.PoolSize == nil || *rebuilt.PoolSize != 5 {
		t.Fatalf("pool was not preserved")
	}
}

func TestValidatePortalMetadataIsIndependent(t *testing.T) {
	tags := map[string]string{"region": "sg"}
	sid, peerType := "peer-id", "portal"
	tunnel := ParseTunnelURL("portal://secret@:2077")
	tunnel.Tags = &tags
	tunnel.Peer = &models.Peer{SID: &sid, Type: &peerType}
	if err := ValidatePortalTunnel(*tunnel); err != nil {
		t.Fatalf("metadata must not affect URL validation: %v", err)
	}
	reparsed := ParseTunnelURL(BuildTunnelURLs(*tunnel))
	if reparsed.Tags != nil || reparsed.Peer != nil {
		t.Fatalf("OpenCtrl metadata must not be serialized into the Portal URL")
	}
}

func TestPortalRejectsConflictingOptions(t *testing.T) {
	conflict := ParseTunnelURL("portal://secret@:2077?socks=127.0.0.1:1080&next=secret@relay.example:2077")
	if err := ValidatePortalTunnel(*conflict); err == nil {
		t.Fatal("socks and next must be mutually exclusive")
	}
}

func TestBuildVectorURL(t *testing.T) {
	portal := ParseTunnelURL("portal://secret@:2077?net=tcp&alpn=now%2Fprivate&rate=100&etar=200&log=warn")
	vector, err := BuildVectorURL(*portal, "portal.example", "127.0.0.1:1080")
	if err != nil {
		t.Fatalf("build Vector URL: %v", err)
	}
	parsed, err := url.Parse(vector)
	if err != nil {
		t.Fatalf("parse Vector URL: %v", err)
	}
	if parsed.Scheme != "nowhere" || parsed.Host != "portal.example:2077" || parsed.User.Username() != "secret" {
		t.Fatalf("unexpected Vector URL: %s", vector)
	}
	query := parsed.Query()
	if query.Get("up") != "tcp" || query.Get("down") != "tcp" || query.Get("pool") != "5" {
		t.Fatalf("tcp carrier mapping is wrong: %s", vector)
	}
	if query.Get("socks") != "127.0.0.1:1080" || query.Get("alpn") != "now/private" {
		t.Fatalf("Vector settings were not preserved: %s", vector)
	}
}

func TestBuildVectorURLPrefersConcreteListenHost(t *testing.T) {
	portal := ParseTunnelURL("portal://secret@192.0.2.10:2077")
	vector, err := BuildVectorURL(*portal, "portal.example", "")
	if err != nil {
		t.Fatalf("build Vector URL: %v", err)
	}

	parsed, err := url.Parse(vector)
	if err != nil {
		t.Fatalf("parse Vector URL: %v", err)
	}
	if parsed.Host != "192.0.2.10:2077" {
		t.Fatalf("host = %q, want concrete listen host", parsed.Host)
	}
	if parsed.Query().Get("socks") != "127.0.0.1:1080" {
		t.Fatalf("default socks listener missing from %s", vector)
	}
}

func TestBuildVectorURLUsesBracketedIPv6ForWildcardListener(t *testing.T) {
	portal := ParseTunnelURL("portal://secret@[::]:2077")
	vector, err := BuildVectorURL(*portal, "[2001:db8::10]", "127.0.0.1:1080")
	if err != nil {
		t.Fatalf("build Vector URL: %v", err)
	}

	parsed, err := url.Parse(vector)
	if err != nil {
		t.Fatalf("parse Vector URL: %v", err)
	}
	if parsed.Host != "[2001:db8::10]:2077" || parsed.Hostname() != "2001:db8::10" {
		t.Fatalf("unexpected IPv6 Vector host: %q", parsed.Host)
	}
}

func TestTunnelToMapOmitsNilMetadata(t *testing.T) {
	updates := TunnelToMap(&models.Tunnel{})
	for _, key := range []string{"tags", "peer", "config_line"} {
		if _, exists := updates[key]; exists {
			t.Fatalf("%s must be omitted when metadata is nil", key)
		}
	}

	tags := map[string]string{"region": "sg"}
	sid := "peer-id"
	withMetadata := TunnelToMap(&models.Tunnel{
		Tags: &tags,
		Peer: &models.Peer{SID: &sid},
	})
	if !reflect.DeepEqual(withMetadata["tags"], `{"region":"sg"}`) {
		t.Fatalf("serialized tags = %#v", withMetadata["tags"])
	}
	if !reflect.DeepEqual(withMetadata["peer"], `{"sid":"peer-id","type":null,"alias":null}`) {
		t.Fatalf("serialized peer = %#v", withMetadata["peer"])
	}
}
