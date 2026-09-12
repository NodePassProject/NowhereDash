package nowhere

import (
	"net/url"
	"strings"
	"testing"

	"NowhereDash/internal/models"
)

func TestV2NativeQueryRoundTrip(t *testing.T) {
	for _, raw := range []string{
		"portal://local@*/tcp:2006?next=upstream%40key@origin.example/udp6:2017&morph=1&mux=1",
		"portal://local@*:2006?socks=user%2Bname:p%40ss%3Aword@proxy.example:1080",
		"portal://local@*:2006?tls=2&crt=%2Fcert+name.pem&key=%2Fkey%20name.pem",
	} {
		t.Run(raw, func(t *testing.T) {
			portal, err := ParsePortalURL(raw)
			if err != nil {
				t.Fatal(err)
			}
			rebuilt := BuildTunnelURLs(*portal)
			if strings.Contains(raw, "next=") {
				if valueOr(portal.Next, "") != "upstream%40key@origin.example/udp6:2017" ||
					!strings.Contains(rebuilt, "next=upstream%40key@origin.example/udp6:2017") || valueOr(portal.Mux, "") != "0" {
					t.Fatalf("next authority or UDP defaults changed: %s", rebuilt)
				}
			}
			if strings.Contains(raw, "socks=") && !strings.Contains(rebuilt, "socks=user%2Bname:p%40ss%3Aword@proxy.example:1080") {
				t.Fatalf("SOCKS credential delimiters were encoded: %s", rebuilt)
			}
			if strings.Contains(raw, "crt=") && (valueOr(portal.CertPath, "") != "/cert+name.pem" || !strings.Contains(rebuilt, "key=%2Fkey%20name.pem")) {
				t.Fatalf("query changed literal plus or space: %s", rebuilt)
			}
			if _, err := ParsePortalURL(rebuilt); err != nil {
				t.Fatalf("rebuilt URL rejected: %v", err)
			}
		})
	}
}

func TestV2QueryFirstRecognizedValue(t *testing.T) {
	raw := "portal://key@*:2000?morph=1&morph=%GG&unknown=%GG&%FF=x&next=none&up=%GG&pin=%FF"
	portal, err := ParsePortalURL(raw)
	if err != nil || valueOr(portal.Morph, "") != "1" {
		t.Fatalf("ignored query entries affected Portal parsing: %v", err)
	}
	for _, raw := range []string{
		"portal://key@*:2000?next=%6eone&socks=%6eone&up=%GG",
		"portal://key@*:2000?next=key@origin.example:2000&sni=&pin=",
		"portal://key@[::ffff:192.0.2.1]/tcp6:2000",
	} {
		if _, err := ParsePortalURL(raw); err != nil {
			t.Errorf("valid native URL rejected: %s: %v", raw, err)
		}
	}
}

func TestV2MatchingConfigUsesDefaultsAndCanonicalNext(t *testing.T) {
	for _, option := range []string{"morph=1", "rate=50", "etar=50", "next=origin.example:2000", "socks=proxy.example:1080"} {
		config := "portal://*:2000?" + option
		if MatchingPortalConfigURL("portal://key@*:2000", &config) != "" {
			t.Errorf("stale option accepted after removal: %s", option)
		}
	}
	command := "portal://key@[2001:0db8::1]/udp:2000/tcp:2000?next=up%40key@origin.example/udp6:3017&mux=1"
	config := "portal://[2001:db8::1]:2000?next=origin.example/udp6:3017&up=udp&down=udp&mux=0"
	parsed := ParseInstanceTunnel(InstanceResult{URL: command, Config: &config})
	if parsed.ConfigLine == nil || *parsed.ConfigLine != config || valueOr(parsed.Next, "") != "up%40key@origin.example/udp6:3017" || valueOr(parsed.Mux, "") != "0" {
		t.Fatalf("canonical config or credentials lost: %+v", parsed)
	}
	command = "portal://key@*:2000?next=up@origin.example/udp:3017/tcp:3017"
	config = "portal://*:2000?next=origin.example:3017"
	if MatchingPortalConfigURL(command, &config) != config {
		t.Fatal("canonical next was treated as stale")
	}
}

func TestV2RejectsInvalidNativeConfiguration(t *testing.T) {
	for _, suffix := range []string{
		"next=key%40origin.example%3A2000", "next=key@origin.example/udp:2000&up=tcp",
		"socks=user%3Apass%40proxy.example%3A1080", "socks=user:@proxy.example:1080",
		"socks=proxy.example:0", "socks=user:p+ass@proxy.example:1080", "socks=proxy.example:65536",
		"rate=2147483648", "etar=2147483648", "log=INFO", "crt=&key=",
		"next=key@origin.example:2000&sni=127.0.0.1", "next=key@origin.example:2000&sni=" + strings.Repeat("a", 254),
	} {
		if _, err := ParsePortalURL("portal://key@*:2000?" + suffix); err == nil {
			t.Errorf("invalid native configuration accepted: %s", suffix)
		}
	}
	portal := ParseTunnelURL("portal://key@*:2000")
	portal.Socks = stringPtr("proxy&rate=50:1080")
	if err := ValidatePortalTunnel(*portal); err == nil {
		t.Fatal("unencoded SOCKS query delimiter accepted")
	}
	for _, host := range []string{"bad%host", "bad\thost", "bad\nhost", "[broken", "broken]", "bad<host"} {
		tunnel := models.Tunnel{Type: models.TunnelTypePortal, SharedKey: stringPtr("key"), ListenHost: host, ListenPort: "2000"}
		if err := ValidatePortalTunnel(tunnel); err == nil {
			t.Errorf("invalid form host accepted: %q", host)
		}
	}
}

func TestV2VectorSOCKSCredentialsUseNativeSyntax(t *testing.T) {
	portal := ParseTunnelURL("portal://key@*:2000")
	vector, err := BuildVectorURL(*portal, "origin.example", "user:p%40ss@127.0.0.1:1080")
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(vector)
	if !strings.Contains(parsed.RawQuery, "socks=user:p%40ss@127.0.0.1:1080") {
		t.Fatalf("Vector credentials were encoded twice: %s", vector)
	}
}
