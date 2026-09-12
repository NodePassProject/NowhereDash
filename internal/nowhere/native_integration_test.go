package nowhere

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"NowhereDash/internal/models"

	"golang.org/x/net/proxy"
)

type nativeOutput struct {
	mu     sync.Mutex
	buffer bytes.Buffer
	ready  chan struct{}
	once   sync.Once
}

func (output *nativeOutput) Write(data []byte) (int, error) {
	output.mu.Lock()
	defer output.mu.Unlock()
	n, err := output.buffer.Write(data)
	if strings.Contains(output.buffer.String(), "STATE=READY") {
		output.once.Do(func() { close(output.ready) })
	}
	return n, err
}

func (output *nativeOutput) String() string {
	output.mu.Lock()
	defer output.mu.Unlock()
	return output.buffer.String()
}

func startNative(t *testing.T, binary, rawURL string) {
	t.Helper()
	output := &nativeOutput{ready: make(chan struct{})}
	command := exec.Command(binary, rawURL)
	command.Dir = t.TempDir()
	command.Stdout, command.Stderr = output, output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = command.Wait(); close(done) }()
	t.Cleanup(func() {
		_ = command.Process.Kill()
		<-done
	})
	select {
	case <-output.ready:
	case <-done:
		t.Fatalf("native startup failed: %s", output.String())
	case <-time.After(10 * time.Second):
		t.Fatalf("native startup timed out: %s", output.String())
	}
}

func nativeFreePort(t *testing.T, network string) string {
	t.Helper()
	var address net.Addr
	if network == "udp" {
		listener, err := net.ListenPacket("udp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		address = listener.LocalAddr()
		_ = listener.Close()
	} else {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		address = listener.Addr()
		_ = listener.Close()
	}
	_, port, _ := net.SplitHostPort(address.String())
	return port
}

func nativePortal(t *testing.T, carrier string) models.Tunnel {
	t.Helper()
	tcp, udp := "", ""
	if carrier != "udp" {
		tcp = nativeFreePort(t, "tcp")
	}
	if carrier != "tcp" {
		udp = nativeFreePort(t, "udp")
	}
	return models.Tunnel{
		Type: models.TunnelTypePortal, ListenHost: "127.0.0.1", TCPPort: &tcp, UDPPort: &udp,
		TCPFamily: "4", UDPFamily: "4", SharedKey: stringPtr("test@shared+key"), Morph: stringPtr("1"), LogLevel: models.LogLevelInfo,
	}
}

// Opt in with the pinned official v2 binary to verify URL builders against
// real TLS, QUIC, Morph, nested credentials and SOCKS authentication.
func TestNativeV2URLIntegration(t *testing.T) {
	binary := os.Getenv("NOWHERE_V2_TEST_BINARY")
	if binary == "" || testing.Short() {
		t.Skip("set NOWHERE_V2_TEST_BINARY to the Nowhere v2.0.0 binary")
	}
	for _, mode := range []string{"tcp", "udp", "next", "socks"} {
		t.Run(mode, func(t *testing.T) {
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, "native-v2-ok")
			}))
			defer target.Close()
			portal := nativePortal(t, mode)
			if mode == "next" || mode == "socks" {
				origin := nativePortal(t, "udp")
				startNative(t, binary, BuildTunnelURLs(origin))
				if mode == "next" {
					portal.Next = stringPtr(url.User(*origin.SharedKey).String() + "@" + EndpointFromTunnel(origin).Canonical())
					portal.Mux = stringPtr("1")
				} else {
					egress := "user%2Bname:p%40ss%3Aword@127.0.0.1:" + nativeFreePort(t, "tcp")
					vector, err := BuildVectorURL(origin, "", egress)
					if err != nil {
						t.Fatal(err)
					}
					startNative(t, binary, vector)
					portal.Socks = &egress
				}
			}
			command := BuildTunnelURLs(portal)
			if _, err := ParsePortalURL(command); err != nil {
				t.Fatal(err)
			}
			startNative(t, binary, command)
			listener := "127.0.0.1:" + nativeFreePort(t, "tcp")
			vector, err := BuildVectorURL(portal, "", "user%2Bname:p%40ss%3Aword@"+listener)
			if err != nil {
				t.Fatal(err)
			}
			startNative(t, binary, vector)
			dialer, err := proxy.SOCKS5("tcp", listener, &proxy.Auth{User: "user+name", Password: "p@ss:word"}, &net.Dialer{Timeout: 5 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			transport := &http.Transport{Dial: dialer.Dial}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
			response, err := client.Get(target.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil || string(body) != "native-v2-ok" {
				t.Fatalf("native relay response = %q, %v", body, err)
			}
		})
	}
}
