package sse

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"NowhereDash/internal/models"

	"gorm.io/gorm"
)

func TestV2ConfigOnlyEventPreservesCredentials(t *testing.T) {
	database := openSSETestDB(t)
	service := testSSEService(database)
	command := "portal://secret@*/tcp4:2006/udp6:2017?tls=2&crt=%2Fcert.pem&key=%2Fkey.pem&morph=1&next=up%40key@origin.example/udp6:3017&log=debug"
	initial := portalEvent("initial", 1, "redacted-portal", command)
	service.ProcessEvent(initial)
	config := "portal://*/tcp4:2006/udp6:2017?tls=2&morph=1&next=origin.example/udp6:3017&up=udp&down=udp&mux=0"
	update := portalEvent("update", 1, "redacted-portal", "")
	update.TimeStamp = initial.TimeStamp.Add(time.Second)
	update.Instance.Config = &config
	service.ProcessEvent(update)
	var stored models.Tunnel
	if err := database.Where("instance_id = ?", "redacted-portal").First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.CommandLine != command || stored.ConfigLine == nil || *stored.ConfigLine != config || stored.SharedKey == nil || *stored.SharedKey != "secret" ||
		stored.Next == nil || *stored.Next != "up%40key@origin.example/udp6:3017" || stored.CertPath == nil || *stored.CertPath != "/cert.pem" || stored.LogLevel != models.LogLevelDebug || stored.Up == nil || *stored.Up != "udp" {
		t.Fatalf("config-only event lost submitted values: %+v", stored)
	}
	staleDelete := portalEvent("delete", 1, "redacted-portal", command)
	staleDelete.TimeStamp = initial.TimeStamp
	service.handleDeleteEvent(staleDelete)
	if err := database.First(&models.Tunnel{}, stored.ID).Error; err != nil {
		t.Fatalf("stale delete removed current tunnel: %v", err)
	}
}

type notifyingWriter struct {
	*httptest.ResponseRecorder
	done chan struct{}
}

func (writer *notifyingWriter) Write(data []byte) (int, error) {
	n, err := writer.ResponseRecorder.Write(data)
	if strings.Contains(string(data), `"logs":"barrier"`) {
		close(writer.done)
	}
	return n, err
}

func TestV2WorkerPreservesEndpointEventOrder(t *testing.T) {
	database := openSSETestDB(t)
	service := testSSEService(database)
	service.disableLogStore = true
	manager := NewManager(nil, service, false)
	t.Cleanup(manager.Close)
	writer := &notifyingWriter{ResponseRecorder: httptest.NewRecorder(), done: make(chan struct{})}
	service.AddClient("ordered-client", writer)
	service.SubscribeToTunnel("ordered-client", "ordered-portal")
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	if err := database.Callback().Create().Before("gorm:create").Register("test:block-first-create", func(_ *gorm.DB) {
		once.Do(func() { close(started); <-release })
	}); err != nil {
		t.Fatal(err)
	}
	var unblock sync.Once
	t.Cleanup(func() { unblock.Do(func() { close(release) }) })
	send := func(payload SSEResp) {
		payload.Time = "2026-09-12T10:00:00Z"
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		manager.eventQueue(1) <- eventJob{endpointID: 1, payload: string(raw)}
	}
	send(portalEvent("create", 1, "ordered-portal", "portal://secret@*/tcp:2000"))
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not start")
	}
	for i := 1; i <= 12; i++ {
		send(portalEvent("update", 1, "ordered-portal", fmt.Sprintf("portal://secret@*/udp6:%d?morph=0", 2000+i)))
	}
	barrier := portalEvent("log", 1, "ordered-portal", "")
	message := "barrier"
	barrier.Logs = &message
	send(barrier)
	unblock.Do(func() { close(release) })
	select {
	case <-writer.done:
	case <-time.After(5 * time.Second):
		t.Fatal("event stream did not drain")
	}
	var stored models.Tunnel
	if err := database.Where("instance_id = ?", "ordered-portal").First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.UDPPort == nil || *stored.UDPPort != "2012" || stored.TCPPort == nil || *stored.TCPPort != "" || stored.Morph == nil || *stored.Morph != "0" {
		t.Fatalf("worker reordered equal-timestamp events: %+v", stored)
	}
}

func TestConcurrentClientSendsProduceCompleteSSEFrames(t *testing.T) {
	writer := httptest.NewRecorder()
	client := &Client{Writer: writer}
	var pending sync.WaitGroup
	for i := 0; i < 32; i++ {
		pending.Add(1)
		go func(index int) {
			defer pending.Done()
			if err := client.Send([]byte(fmt.Sprintf(`{"sequence":%d}`, index))); err != nil {
				t.Error(err)
			}
		}(i)
	}
	pending.Wait()
	frames := strings.Split(strings.TrimSuffix(writer.Body.String(), "\n\n"), "\n\n")
	if len(frames) != 32 {
		t.Fatalf("SSE frame count = %d", len(frames))
	}
	for _, frame := range frames {
		if !json.Valid([]byte(strings.TrimPrefix(frame, "data: "))) {
			t.Fatalf("corrupted SSE frame: %s", frame)
		}
	}
	client.Close()
	if err := client.Send([]byte(`{}`)); err == nil {
		t.Fatal("closed client accepted a write")
	}
}
