package sse

import (
	"bytes"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"NowhereDash/internal/endpoint"
	"NowhereDash/internal/models"
	"NowhereDash/internal/nowhere"
	tunnels "NowhereDash/internal/tunnel"
)

type controllerOutput struct {
	mu     sync.Mutex
	buffer bytes.Buffer
	key    chan string
	once   sync.Once
}

var controllerKeyPattern = regexp.MustCompile(`API key created: ([A-Za-z0-9_-]+)`)

func (output *controllerOutput) Write(data []byte) (int, error) {
	output.mu.Lock()
	defer output.mu.Unlock()
	n, err := output.buffer.Write(data)
	if match := controllerKeyPattern.FindStringSubmatch(output.buffer.String()); len(match) == 2 {
		output.once.Do(func() { output.key <- match[1] })
	}
	return n, err
}

func controllerFreePort(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	_ = listener.Close()
	return port
}

func awaitControllerState(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("timed out waiting for OpenCtrl/SSE reconciliation")
}

// Both executables are copied or run from temporary paths; no saved dashboard
// database, controller configuration or existing node is used.
func TestOpenCtrlV2FormAndSSEIntegration(t *testing.T) {
	controllerBinary, runtimeBinary := os.Getenv("OPENCTRL_TEST_BINARY"), os.Getenv("NOWHERE_V2_TEST_BINARY")
	if controllerBinary == "" || runtimeBinary == "" || testing.Short() {
		t.Skip("set OPENCTRL_TEST_BINARY and NOWHERE_V2_TEST_BINARY for live API/SSE verification")
	}
	directory := t.TempDir()
	binary, err := os.ReadFile(controllerBinary)
	if err != nil {
		t.Fatal(err)
	}
	localBinary := filepath.Join(directory, "openctrl")
	if err := os.WriteFile(localBinary, binary, 0700); err != nil {
		t.Fatal(err)
	}
	port := controllerFreePort(t)
	output := &controllerOutput{key: make(chan string, 1)}
	process := exec.Command(localBinary, "master://127.0.0.1:"+port+"?bin="+runtimeBinary+"&tls=0")
	process.Dir, process.Stdout, process.Stderr = directory, output, output
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = process.Wait(); close(done) }()
	t.Cleanup(func() {
		_ = process.Process.Signal(os.Interrupt)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = process.Process.Kill()
			<-done
		}
	})
	var apiKey string
	select {
	case apiKey = <-output.key:
	case <-done:
		t.Fatal("OpenCtrl exited before creating the test API key")
	case <-time.After(10 * time.Second):
		t.Fatal("OpenCtrl did not create the test API key")
	}
	database := openSSETestDB(t)
	if err := database.AutoMigrate(&models.EndpointTrafficCursor{}, &models.PortalSubscription{}, &models.PortalSubscriptionTunnel{}, &models.PortalSubscriptionExternalNode{}); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	node := models.Endpoint{Name: "live-v2-test", URL: "http://127.0.0.1:" + port, APIPath: "/api/v2", APIKey: apiKey}
	if err := database.Create(&node).Error; err != nil {
		t.Fatal(err)
	}
	cacheKey := strconv.FormatInt(node.ID, 10)
	nowhere.GetCache().Set(cacheKey, node.URL+node.APIPath, apiKey)
	t.Cleanup(func() { nowhere.GetCache().Delete(cacheKey) })
	awaitControllerState(t, func() bool { _, err := nowhere.GetInstances(node.ID); return err == nil })
	service := testSSEService(database)
	service.endpointService = endpoint.NewService(database)
	service.disableLogStore = true
	manager := NewManager(sqlDB, service, false)
	service.SetManager(manager)
	t.Cleanup(manager.Close)
	if err := manager.ConnectEndpoint(node.ID, node.URL, node.APIPath, apiKey); err != nil {
		t.Fatal(err)
	}

	tcp, udp, morph := controllerFreePort(t), controllerFreePort(t), "1"
	req := tunnels.PortalRequest{
		EndpointID: node.ID, Name: "form-created-v2", ListenHost: "127.0.0.1", SharedKey: "test@key+value",
		TCPPort: &tcp, UDPPort: &udp, TCPFamily: "4", UDPFamily: "4", Morph: &morph,
		Next: "up%40key@127.0.0.1/udp4:3017", Mux: "1", LogLevel: models.LogLevelInfo,
	}
	tunnelService := tunnels.NewService(database)
	created, err := tunnelService.CreatePortal(req)
	if err != nil {
		t.Fatal(err)
	}
	if created.InstanceID == nil {
		t.Fatal("created Portal has no instance ID")
	}
	t.Cleanup(func() { _ = nowhere.DeleteInstance(node.ID, *created.InstanceID) })
	var stored models.Tunnel
	t.Cleanup(func() {
		if t.Failed() {
			remote, _ := nowhere.GetInstance(node.ID, *created.InstanceID)
			diagnostic, _ := json.Marshal(map[string]interface{}{"stored": stored, "remote": remote})
			t.Logf("live reconciliation state: %s", diagnostic)
		}
	})
	awaitControllerState(t, func() bool {
		stored = models.Tunnel{}
		return database.First(&stored, created.ID).Error == nil && stored.Name == req.Name &&
			stored.Status == models.TunnelStatusRunning && stored.LastEventTime.Valid && strings.Contains(stored.CommandLine, "next=up%40key@127.0.0.1/udp4:3017")
	})
	if stored.SharedKey == nil || *stored.SharedKey != req.SharedKey || stored.Next == nil || *stored.Next != req.Next || stored.Up == nil || *stored.Up != "udp" || stored.Mux == nil || *stored.Mux != "0" {
		t.Fatalf("live SSE lost form values: %+v", stored)
	}
	disabled, morphOff := "", "0"
	req.TCPPort, req.Morph, req.Next = &disabled, &morphOff, "none"
	req.Socks = "user:p%40ss@127.0.0.1:1080"
	updated, err := tunnelService.UpdatePortal(created.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	awaitControllerState(t, func() bool {
		stored = models.Tunnel{}
		return database.First(&stored, updated.ID).Error == nil && stored.Status == models.TunnelStatusRunning &&
			strings.Contains(stored.CommandLine, "morph=0") && strings.Contains(stored.CommandLine, "socks=user:p%40ss@127.0.0.1:1080") &&
			stored.LastEventTime.Time.After(updated.LastEventTime.Time)
	})
	if stored.TCPPort == nil || *stored.TCPPort != "" || stored.UDPPort == nil || *stored.UDPPort != udp || stored.Morph == nil || *stored.Morph != "0" || stored.Next == nil || *stored.Next != "none" || stored.Socks == nil || *stored.Socks != req.Socks {
		t.Fatalf("live SSE reverted disabled form fields: %+v", stored)
	}
	if _, err := nowhere.RenameInstance(node.ID, *created.InstanceID, "sse-updated-name"); err != nil {
		t.Fatal(err)
	}
	awaitControllerState(t, func() bool {
		return database.First(&stored, updated.ID).Error == nil && stored.Name == "sse-updated-name"
	})
}
