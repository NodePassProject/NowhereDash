package nowhere

import (
	"net/url"
	"reflect"
	"testing"

	"NowhereDash/internal/models"
)

func TestV2EndpointRoundTrip(t *testing.T) {
	for _, test := range []struct{ input, canonical, network string }{
		{"portal://key@:2000", "*:2000", "mix"},
		{"portal://key@*/udp:2017/tcp:2006", "*/tcp:2006/udp:2017", "mix"},
		{"portal://key@*/tcp:2000/udp:2000", "*:2000", "mix"},
		{"portal://key@*/tcp4:2006/udp6:2017", "*/tcp4:2006/udp6:2017", "mix"},
		{"portal://key@[2001:db8::1]/udp6:2017", "[2001:db8::1]/udp6:2017", "udp"},
		{"portal://key@192.0.2.1/tcp4:2006", "192.0.2.1/tcp4:2006", "tcp"},
		{"portal://key@*:2000?net=tcp&alpn=custom", "*:2000", "mix"},
	} {
		t.Run(test.input, func(t *testing.T) {
			portal, err := ParsePortalURL(test.input)
			if err != nil {
				t.Fatal(err)
			}
			endpoint := EndpointFromTunnel(*portal)
			if endpoint.Canonical() != test.canonical || endpoint.Network() != test.network {
				t.Fatalf("unexpected endpoint: %+v", endpoint)
			}
			rebuilt := BuildTunnelURLs(*portal)
			reparsed, err := ParsePortalURL(rebuilt)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(endpoint, EndpointFromTunnel(*reparsed)) {
				t.Fatalf("endpoint changed: %s", rebuilt)
			}
			parsed, _ := url.Parse(rebuilt)
			if parsed.Query().Has("net") || parsed.Query().Has("alpn") || valueOr(reparsed.ALPN, "") != "nw2" {
				t.Fatalf("removed parameters survived: %s", rebuilt)
			}
		})
	}
}

func TestV2InvalidEndpointsAndOptions(t *testing.T) {
	for _, input := range []string{
		"portal://key@host:2000/tcp:2006", "portal://key@host/tcp:2006/", "portal://key@host/tcp:2006/tcp6:2006",
		"portal://key@host/sctp:2000", "portal://key@192.0.2.1/tcp6:2006", "portal://key@[::1]/udp4:2000",
		"portal://key@host/tcp:0", "portal://key@host/tcp:65536", "portal://key@host/tcp:+2000", "portal://key@host/tcp:%32",
		"portal://key@host/tcp:2000/../udp:2000", "portal://key@host/%2e/tcp:2000", "portal://key@/tcp:2000",
		"portal://key:password@host:2000", "portal://key@host:2000#fragment", "portal://key@host:2000?morph=",
		"portal://key@host:2000?morph=2", "portal://key@host:2000?rate=invalid", "portal://key@host:2000?etar=-1",
		"portal://key@host:2000?next=origin@host:2000&up=", "portal://key@host:2000?next=origin@host:2000&mux=",
		"portal://key@host:2000?next=origin@*/tcp:2000", "portal://key@host:2000?next=origin@host/udp:2000&up=tcp",
		"portal://key@host:2000?next=origin@host/tcp:2000&down=mix", "portal://key@host:2000?next=origin@host:2000%3Fmorph%3D1",
	} {
		t.Run(input, func(t *testing.T) {
			if _, err := ParsePortalURL(input); err == nil {
				t.Fatal("invalid v2 URL accepted")
			}
		})
	}
}

func TestV2NextDefaultsAndVectorPreserveIndependentEndpoint(t *testing.T) {
	portal, err := ParsePortalURL("portal://local@*/tcp4:2006/udp6:2017?morph=1&next=upstream@origin.example/udp6:3017")
	if err != nil {
		t.Fatal(err)
	}
	if *portal.Up != "udp" || *portal.Down != "udp" {
		t.Fatal("next should default to its only carrier")
	}
	vector, err := BuildVectorURL(*portal, "entry.example", "")
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(vector)
	if parsed.Host != "entry.example" || parsed.Path != "/tcp4:2006/udp6:2017" || parsed.Query().Get("morph") != "1" || parsed.Query().Get("up") != "tcp" {
		t.Fatalf("invalid vector: %s", vector)
	}
	if _, err := BuildVectorURL(*portal, "192.0.2.1", ""); err == nil {
		t.Fatal("address-family conflict in public host must fail")
	}
	config := "portal://*/udp6:2017/tcp4:2006?morph=1&next=origin.example/udp6:3017&up=udp&down=udp&mux=0"
	merged := ParseInstanceTunnel(InstanceResult{URL: portal.CommandLine, Config: &config})
	if merged.ConfigLine == nil || *merged.ConfigLine != config || *merged.Next != *portal.Next {
		t.Fatal("canonical runtime endpoint or next credentials were lost")
	}
	stale := "portal://*/tcp4:2006/udp6:2018?morph=1"
	if MatchingPortalConfigURL(portal.CommandLine, &stale) != "" {
		t.Fatal("stale UDP port was accepted")
	}
}

func TestV2MapClearsDisabledCarrierAndMorph(t *testing.T) {
	portal := ParseTunnelURL("portal://key@*/udp6:2017?morph=0")
	updates := TunnelToMap(portal)
	if valueOr(updates["tcp_port"].(*string), "") != "" || *updates["udp_port"].(*string) != "2017" || *updates["morph"].(*string) != "0" {
		t.Fatalf("incomplete updates: %#v", updates)
	}
	legacy := models.Tunnel{ListenPort: "2077", Network: stringPtr("udp")}
	if endpoint := EndpointFromTunnel(legacy); endpoint.TCPPort != "" || endpoint.UDPPort != "2077" {
		t.Fatalf("legacy entity lost its listener: %+v", endpoint)
	}
}
