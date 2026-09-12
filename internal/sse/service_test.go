package sse

import (
	"encoding/json"
	"testing"
	"time"

	"NowhereDash/internal/models"
	"NowhereDash/internal/nowhere"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func openSSETestDB(t *testing.T) *gorm.DB {
	t.Helper()
	database, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := database.AutoMigrate(&models.Endpoint{}, &models.Tunnel{}, &models.TunnelOperationLog{}); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	return database
}

func testSSEService(database *gorm.DB) *Service {
	return &Service{
		clients:    make(map[string]*Client),
		tunnelSubs: make(map[string]map[string]*Client),
		db:         database,
	}
}

func portalEvent(eventType string, endpointID int64, instanceID, rawURL string) SSEResp {
	var payload SSEResp
	payload.Type = eventType
	payload.EndpointID = endpointID
	payload.TimeStamp = time.Now()
	payload.Instance = nowhere.InstanceResult{
		ID:     instanceID,
		Type:   string(models.TunnelTypePortal),
		Status: string(models.TunnelStatusRunning),
		URL:    rawURL,
	}
	return payload
}

func TestPortalDeletionPreservesEndpointTraffic(t *testing.T) {
	database := openSSETestDB(t)
	if err := database.AutoMigrate(&models.EndpointTrafficCursor{}, &models.PortalSubscription{}, &models.PortalSubscriptionTunnel{}, &models.PortalSubscriptionExternalNode{}); err != nil {
		t.Fatal(err)
	}
	ep := models.Endpoint{Name: "traffic", URL: "http://traffic", APIPath: "/api/v2", APIKey: "test"}
	if err := database.Create(&ep).Error; err != nil {
		t.Fatal(err)
	}
	service := testSSEService(database)
	base := time.Now()
	for i, eventType := range []string{"initial", "update", "delete"} {
		payload := portalEvent(eventType, ep.ID, "accounted-portal", "portal://secret@:2077")
		payload.TimeStamp = base.Add(time.Duration(i) * time.Second)
		payload.Instance.TCPRx = 100
		if i > 0 {
			payload.Instance.TCPRx = 175
		}
		service.ProcessEvent(payload)
	}
	var count int64
	if err := database.Model(&models.Tunnel{}).Where("endpoint_id = ?", ep.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("Portal was not deleted: %d", count)
	}
	if err := database.First(&ep, ep.ID).Error; err != nil {
		t.Fatal(err)
	}
	if ep.TrafficTotal != 175 || ep.TrafficUsed != 175 {
		t.Fatalf("traffic after SSE deletion = total %d, period %d", ep.TrafficTotal, ep.TrafficUsed)
	}
}

func TestPortalEventsWithoutMetadataPreserveStoredMetadata(t *testing.T) {
	for _, eventType := range []string{"create", "update"} {
		t.Run(eventType, func(t *testing.T) {
			database := openSSETestDB(t)
			tags := map[string]string{"region": "sg"}
			peerSID, peerType := "peer-1", "portal"
			instanceID := "portal-1"
			tunnel := models.Tunnel{
				Name:        "portal",
				EndpointID:  1,
				InstanceID:  &instanceID,
				Type:        models.TunnelTypePortal,
				Status:      models.TunnelStatusRunning,
				ListenPort:  "2077",
				CommandLine: "portal://secret@:2077",
				Tags:        &tags,
				Peer:        &models.Peer{SID: &peerSID, Type: &peerType},
			}
			if err := database.Create(&tunnel).Error; err != nil {
				t.Fatalf("create tunnel: %v", err)
			}

			service := testSSEService(database)
			service.ProcessEvent(portalEvent(eventType, 1, instanceID, "portal://secret@:2077"))

			var stored models.Tunnel
			if err := database.First(&stored, tunnel.ID).Error; err != nil {
				t.Fatalf("load tunnel: %v", err)
			}
			if stored.Tags == nil || (*stored.Tags)["region"] != "sg" {
				t.Fatalf("tags = %#v, want stored metadata preserved", stored.Tags)
			}
			if stored.Peer == nil || stored.Peer.SID == nil || *stored.Peer.SID != peerSID {
				t.Fatalf("peer = %#v, want stored metadata preserved", stored.Peer)
			}
		})
	}
}

func TestSSEInstanceDecodesNowhereURLAliases(t *testing.T) {
	var payload SSEResp
	raw := `{"type":"update","instance":{"id":"portal-1","type":"portal","status":"running","commandURL":"portal://secret@:2077","configURL":"portal://secret@:2077?rate=0"}}`
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("decode SSE payload: %v", err)
	}
	if payload.Instance.URL != "portal://secret@:2077" {
		t.Fatalf("command URL = %q", payload.Instance.URL)
	}
	if payload.Instance.Config == nil || *payload.Instance.Config != "portal://secret@:2077?rate=0" {
		t.Fatalf("config URL = %#v", payload.Instance.Config)
	}
}

func TestPortalUpdateUsesExpandedConfigURL(t *testing.T) {
	database := openSSETestDB(t)
	instanceID := "portal-1"
	tags := map[string]string{"region": "sg"}
	commandURL := "portal://runtime@*/tcp:2077"
	configURL := "portal://*/tcp:2077?tls=1&morph=0&rate=0&etar=0&dial=auto&socks=none&next=none"
	tunnel := models.Tunnel{
		Name: "portal", EndpointID: 1, InstanceID: &instanceID,
		Type: models.TunnelTypePortal, Status: models.TunnelStatusRunning,
		ListenPort: "2077", CommandLine: commandURL, Tags: &tags,
	}
	if err := database.Create(&tunnel).Error; err != nil {
		t.Fatalf("create tunnel: %v", err)
	}

	payload := portalEvent("update", 1, instanceID, commandURL)
	payload.Instance.Config = &configURL
	testSSEService(database).ProcessEvent(payload)

	var stored models.Tunnel
	if err := database.First(&stored, tunnel.ID).Error; err != nil {
		t.Fatalf("load tunnel: %v", err)
	}
	if stored.ConfigLine == nil || *stored.ConfigLine != configURL || stored.SharedKey == nil ||
		*stored.SharedKey != "runtime" || stored.Network == nil || *stored.Network != "tcp" ||
		stored.Rate == nil || *stored.Rate != 0 {
		t.Fatalf("expanded runtime config was not stored: %+v", stored)
	}
	if stored.Tags == nil || (*stored.Tags)["region"] != "sg" {
		t.Fatalf("metadata was overwritten: %#v", stored.Tags)
	}
}

func TestEmptyConfigEventPreservesStoredConfigURL(t *testing.T) {
	database := openSSETestDB(t)
	instanceID := "portal-1"
	commandURL := "portal://secret@:2077?net=tcp"
	configURL := "portal://:2077?net=tcp&tls=1&alpn=now%2F1&rate=0&etar=0&dial=auto&socks=none&next=none"
	eventTime := time.Now().Add(-time.Minute)
	tunnel := models.Tunnel{
		Name: "portal", EndpointID: 1, InstanceID: &instanceID,
		Type: models.TunnelTypePortal, Status: models.TunnelStatusRunning,
		ListenPort: "2077", CommandLine: commandURL, ConfigLine: &configURL,
		LastEventTime: models.NullTime{Time: eventTime, Valid: true},
	}
	if err := database.Create(&tunnel).Error; err != nil {
		t.Fatalf("create tunnel: %v", err)
	}

	payload := portalEvent("update", 1, instanceID, commandURL)
	emptyConfig := ""
	payload.Instance.Config = &emptyConfig
	testSSEService(database).ProcessEvent(payload)

	var stored models.Tunnel
	if err := database.First(&stored, tunnel.ID).Error; err != nil {
		t.Fatalf("load tunnel: %v", err)
	}
	if stored.ConfigLine == nil || *stored.ConfigLine != configURL {
		t.Fatalf("empty SSE config overwrote stored config: %#v", stored.ConfigLine)
	}
}

func TestOlderPortalEventCannotOverwriteNewerState(t *testing.T) {
	database := openSSETestDB(t)
	instanceID := "portal-1"
	newCommand := "portal://secret@:2077?net=tcp"
	newConfig := "portal://secret@:2077?net=tcp&rate=0"
	newTags := map[string]string{"generation": "new"}
	newerTime := time.Now()
	tunnel := models.Tunnel{
		Name: "portal", EndpointID: 1, InstanceID: &instanceID,
		Type: models.TunnelTypePortal, Status: models.TunnelStatusRunning,
		ListenPort: "2077", CommandLine: newCommand, ConfigLine: &newConfig,
		Tags: &newTags, LastEventTime: models.NullTime{Time: newerTime, Valid: true},
	}
	if err := database.Create(&tunnel).Error; err != nil {
		t.Fatalf("create tunnel: %v", err)
	}

	oldCommand := "portal://secret@:2077?net=udp"
	oldConfig := "portal://secret@:2077?net=udp&rate=99"
	oldTags := map[string]string{"generation": "old"}
	payload := portalEvent("update", 1, instanceID, oldCommand)
	payload.TimeStamp = newerTime.Add(-time.Minute)
	payload.Instance.Config = &oldConfig
	payload.Instance.Status = string(models.TunnelStatusStopped)
	payload.Instance.Meta = &nowhere.Meta{Tags: &oldTags}
	testSSEService(database).ProcessEvent(payload)

	var stored models.Tunnel
	if err := database.First(&stored, tunnel.ID).Error; err != nil {
		t.Fatalf("load tunnel: %v", err)
	}
	if stored.CommandLine != newCommand || stored.ConfigLine == nil || *stored.ConfigLine != newConfig ||
		stored.Status != models.TunnelStatusRunning || stored.Tags == nil || (*stored.Tags)["generation"] != "new" ||
		!stored.LastEventTime.Valid || !stored.LastEventTime.Time.Equal(newerTime) {
		t.Fatalf("older event overwrote newer state: %+v", stored)
	}
}

func TestNonPortalUpdateIsIgnored(t *testing.T) {
	database := openSSETestDB(t)
	instanceID := "portal-1"
	tunnel := models.Tunnel{
		Name:        "unchanged",
		EndpointID:  1,
		InstanceID:  &instanceID,
		Type:        models.TunnelTypePortal,
		Status:      models.TunnelStatusRunning,
		ListenPort:  "2077",
		CommandLine: "portal://secret@:2077",
	}
	if err := database.Create(&tunnel).Error; err != nil {
		t.Fatalf("create tunnel: %v", err)
	}

	historyWorker := &HistoryWorker{dataInputChan: make(chan MonitoringData, 1)}
	service := testSSEService(database)
	service.historyWorker = historyWorker
	payload := portalEvent("update", 1, instanceID, "portal://changed@:9999")
	payload.Instance.Type = "client"
	changed := "changed"
	payload.Instance.Alias = &changed
	service.ProcessEvent(payload)

	if got := len(historyWorker.dataInputChan); got != 0 {
		t.Fatalf("history queue length = %d, want 0", got)
	}
	var stored models.Tunnel
	if err := database.First(&stored, tunnel.ID).Error; err != nil {
		t.Fatalf("load tunnel: %v", err)
	}
	if stored.Name != "unchanged" || stored.ListenPort != "2077" {
		t.Fatalf("non-portal event changed tunnel: %#v", stored)
	}
}

func TestEndpointTunnelCountIncludesOnlyPortals(t *testing.T) {
	database := openSSETestDB(t)
	endpoint := models.Endpoint{Name: "master", URL: "http://example.test", APIPath: "/api/v2", APIKey: "secret"}
	if err := database.Create(&endpoint).Error; err != nil {
		t.Fatalf("create endpoint: %v", err)
	}
	portalID, legacyID := "portal-1", "legacy-1"
	rows := []models.Tunnel{
		{Name: "portal", EndpointID: endpoint.ID, InstanceID: &portalID, Type: models.TunnelTypePortal, ListenPort: "2077", CommandLine: "portal://secret@:2077"},
		{Name: "legacy", EndpointID: endpoint.ID, InstanceID: &legacyID, Type: models.TunnelType("unsupported"), ListenPort: "2088", CommandLine: "unsupported://legacy"},
	}
	if err := database.Create(&rows).Error; err != nil {
		t.Fatalf("create tunnels: %v", err)
	}

	testSSEService(database).updateEndpointTunnelCount(endpoint.ID)
	if err := database.First(&endpoint, endpoint.ID).Error; err != nil {
		t.Fatalf("reload endpoint: %v", err)
	}
	if endpoint.TunnelCount != 1 {
		t.Fatalf("tunnel count = %d, want 1", endpoint.TunnelCount)
	}
}

func TestV2PortalEventsPreserveMissingConfigAndClearDisabledOptions(t *testing.T) {
	for _, eventType := range []string{"initial", "create", "update"} {
		t.Run(eventType, func(t *testing.T) {
			database := openSSETestDB(t)
			service := testSSEService(database)
			command := "portal://secret@*/tcp4:2006/udp6:2017?morph=1&next=up%40key@origin.example/udp6:3017"
			initial := portalEvent("initial", 1, "v2-portal", command)
			alias, restart := "saved name", true
			initial.Instance.Alias, initial.Instance.Restart = &alias, &restart
			service.ProcessEvent(initial)
			partial := portalEvent(eventType, 1, "v2-portal", "")
			partial.TimeStamp = initial.TimeStamp.Add(time.Second)
			partial.Instance.TCPRx = 123
			service.ProcessEvent(partial)
			var stored models.Tunnel
			if err := database.Where("instance_id = ?", "v2-portal").First(&stored).Error; err != nil {
				t.Fatal(err)
			}
			if stored.Type != models.TunnelTypePortal || stored.CommandLine != command || stored.TCPPort == nil || *stored.TCPPort != "2006" ||
				stored.Morph == nil || *stored.Morph != "1" || stored.SharedKey == nil || *stored.SharedKey != "secret" || stored.Name != alias || stored.Restart == nil || !*stored.Restart || stored.TCPRx != 123 {
				t.Fatalf("partial event erased stored configuration: %+v", stored)
			}
			updated := portalEvent("update", 1, "v2-portal", "portal://secret@*/udp6:2017?morph=0")
			updated.TimeStamp = partial.TimeStamp.Add(time.Second)
			service.ProcessEvent(updated)
			if err := database.First(&stored, stored.ID).Error; err != nil {
				t.Fatal(err)
			}
			if stored.TCPPort == nil || *stored.TCPPort != "" || stored.UDPPort == nil || *stored.UDPPort != "2017" || stored.Morph == nil || *stored.Morph != "0" || stored.Next == nil || *stored.Next != "none" {
				t.Fatalf("disabled options survived SSE update: %+v", stored)
			}
		})
	}
}
