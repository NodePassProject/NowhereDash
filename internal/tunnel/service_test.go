package tunnel

import (
	"testing"
	"time"

	"NowhereDash/internal/models"
	"NowhereDash/internal/nowhere"
	"NowhereDash/internal/subscription"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestSubmittedMetadataOverridesStaleInstanceResponse(t *testing.T) {
	staleTags := map[string]string{"region": "old"}
	staleSID, peerType := "old-peer", "portal"
	tunnel := models.Tunnel{}
	applyInstanceState(&tunnel, nowhere.InstanceResult{
		Meta: &nowhere.Meta{
			Tags: &staleTags,
			Peer: &models.Peer{SID: &staleSID, Type: &peerType},
		},
	})

	submittedTags := map[string]string{"region": "sg"}
	submittedSID := "submitted-peer"
	submittedPeer := &models.Peer{SID: &submittedSID, Type: &peerType}
	applySubmittedMetadata(&tunnel, &submittedTags, submittedPeer)

	if tunnel.Tags == nil || (*tunnel.Tags)["region"] != "sg" {
		t.Fatalf("tags = %#v, want submitted tags", tunnel.Tags)
	}
	if tunnel.Peer == nil || tunnel.Peer.SID == nil || *tunnel.Peer.SID != submittedSID {
		t.Fatalf("peer = %#v, want submitted peer", tunnel.Peer)
	}
}

func TestApplyInstanceStateUsesConfigURLAndPreservesMetadata(t *testing.T) {
	tags := map[string]string{"region": "sg"}
	peerSID := "peer-1"
	commandURL := "portal://runtime@:2077?net=tcp"
	configURL := "portal://:2077?net=tcp&tls=1&alpn=now%2F1&rate=0&etar=0&dial=auto&socks=none&next=none"
	tunnel := models.Tunnel{
		CommandLine: commandURL,
		Tags:        &tags,
		Peer:        &models.Peer{SID: &peerSID},
	}

	applyInstanceState(&tunnel, nowhere.InstanceResult{
		ID:     "portal-1",
		URL:    commandURL,
		Config: &configURL,
	})

	if tunnel.ConfigLine == nil || *tunnel.ConfigLine != configURL || tunnel.CommandLine != commandURL {
		t.Fatalf("URLs = command:%q config:%#v", tunnel.CommandLine, tunnel.ConfigLine)
	}
	if tunnel.SharedKey == nil || *tunnel.SharedKey != "runtime" || tunnel.Network == nil || *tunnel.Network != "tcp" ||
		tunnel.Rate == nil || *tunnel.Rate != 0 {
		t.Fatalf("expanded runtime config was not applied: %+v", tunnel)
	}
	if tunnel.Tags == nil || (*tunnel.Tags)["region"] != "sg" || tunnel.Peer == nil ||
		tunnel.Peer.SID == nil || *tunnel.Peer.SID != peerSID {
		t.Fatalf("metadata was overwritten: tags=%#v peer=%#v", tunnel.Tags, tunnel.Peer)
	}
}

func TestPersistCreatedPortalReconcilesSSEInsert(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Endpoint{}, &models.Tunnel{}); err != nil {
		t.Fatal(err)
	}
	endpoint := models.Endpoint{Name: "master", URL: "master://example:10101", APIPath: "/api/v2", APIKey: "key"}
	if err := db.Create(&endpoint).Error; err != nil {
		t.Fatal(err)
	}

	instanceID := "portal-race"
	oldKey, oldNetwork := "sse-key", "tcp"
	oldTags := map[string]string{"region": "old"}
	oldSID, peerType := "sse-peer", "portal"
	restartFalse := false
	tcps, udps, pool, ping := int64(3), int64(4), int64(5), int64(6)
	configLine := "runtime config"
	eventTime := time.Date(2026, time.August, 23, 8, 30, 0, 0, time.UTC)
	sseRow := models.Tunnel{
		Name: "SSE row", EndpointID: endpoint.ID, InstanceID: &instanceID,
		Type: models.TunnelTypePortal, Status: models.TunnelStatusRunning,
		ListenHost: "0.0.0.0", ListenPort: "10001", TLSMode: models.TLS1,
		CommandLine: "portal://sse-key@:10001", SharedKey: &oldKey, Network: &oldNetwork,
		Restart: &restartFalse, Tags: &oldTags, Peer: &models.Peer{SID: &oldSID, Type: &peerType},
		TCPRx: 101, TCPTx: 102, UDPRx: 103, UDPTx: 104,
		TCPs: &tcps, UDPs: &udps, Pool: &pool, Ping: &ping,
		ConfigLine: &configLine, LastEventTime: models.NullTime{Time: eventTime, Valid: true},
		Sorts: 1,
	}
	if err := db.Create(&sseRow).Error; err != nil {
		t.Fatal(err)
	}

	newKey, newNetwork, alpn := "api-key", "mix", "now/1"
	newTags := map[string]string{"region": "sg"}
	newSID := "api-peer"
	restartTrue := true
	rate, etar, poolSize := int64(100), int64(200), int64(8)
	incoming := models.Tunnel{
		Name: "API Portal", EndpointID: endpoint.ID, InstanceID: &instanceID,
		Type: models.TunnelTypePortal, Status: models.TunnelStatusStopped,
		ListenHost: "::", ListenPort: "20001", TLSMode: models.TLS1,
		CommandLine: "portal://api-key@[::]:20001", SharedKey: &newKey, Network: &newNetwork,
		ALPN: &alpn, Rate: &rate, Etar: &etar, PoolSize: &poolSize,
		Restart: &restartTrue, Tags: &newTags, Peer: &models.Peer{SID: &newSID, Type: &peerType},
		EnableLogStore: true, Sorts: 42,
		TCPRx: 901, TCPTx: 902, UDPRx: 903, UDPTx: 904,
	}

	if err := NewService(db).persistCreatedPortal(&incoming); err != nil {
		t.Fatalf("persistCreatedPortal: %v", err)
	}
	if incoming.ID != sseRow.ID {
		t.Fatalf("canonical ID = %d, want pre-existing ID %d", incoming.ID, sseRow.ID)
	}

	var count int64
	if err := db.Model(&models.Tunnel{}).
		Where("endpoint_id = ? AND instance_id = ?", endpoint.ID, instanceID).
		Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("row count = %d, want 1", count)
	}

	var stored models.Tunnel
	if err := db.First(&stored, sseRow.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Name != "API Portal" || stored.ListenHost != "::" || stored.ListenPort != "20001" ||
		stored.CommandLine != "portal://api-key@[::]:20001" || stored.SharedKey == nil || *stored.SharedKey != newKey ||
		stored.Network == nil || *stored.Network != newNetwork || stored.ALPN == nil || *stored.ALPN != alpn ||
		stored.Rate == nil || *stored.Rate != rate || stored.Etar == nil || *stored.Etar != etar ||
		stored.PoolSize == nil || *stored.PoolSize != poolSize || stored.Restart == nil || !*stored.Restart ||
		!stored.EnableLogStore || stored.Sorts != 42 {
		t.Fatalf("API configuration was not applied: %+v", stored)
	}
	if stored.Tags == nil || (*stored.Tags)["region"] != "sg" || stored.Peer == nil ||
		stored.Peer.SID == nil || *stored.Peer.SID != newSID {
		t.Fatalf("API metadata was not applied: tags=%#v peer=%#v", stored.Tags, stored.Peer)
	}
	if stored.Status != sseRow.Status || stored.TCPRx != sseRow.TCPRx || stored.TCPTx != sseRow.TCPTx ||
		stored.UDPRx != sseRow.UDPRx || stored.UDPTx != sseRow.UDPTx ||
		stored.TCPs == nil || *stored.TCPs != tcps || stored.UDPs == nil || *stored.UDPs != udps ||
		stored.Pool == nil || *stored.Pool != pool || stored.Ping == nil || *stored.Ping != ping {
		t.Fatalf("SSE runtime state was overwritten: %+v", stored)
	}
	if stored.ConfigLine == nil || *stored.ConfigLine != configLine || !stored.LastEventTime.Valid ||
		!stored.LastEventTime.Time.Equal(eventTime) {
		t.Fatalf("SSE event state was overwritten: config=%#v event=%+v", stored.ConfigLine, stored.LastEventTime)
	}
}

func TestDeleteTunnelAccountsSubscriptionTrafficBeforeRemovingLink(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&models.Endpoint{}, &models.Tunnel{}, &models.TunnelOperationLog{}, &models.TunnelGroup{},
		&models.PortalSubscription{}, &models.PortalSubscriptionTunnel{},
	); err != nil {
		t.Fatal(err)
	}
	endpoint := models.Endpoint{Name: "master", URL: "master://example:10101", APIPath: "/api/v2", APIKey: "key"}
	if err := db.Create(&endpoint).Error; err != nil {
		t.Fatal(err)
	}
	key, network := "secret", "tcp"
	portal := models.Tunnel{
		Name: "portal", EndpointID: endpoint.ID, Type: models.TunnelTypePortal,
		Status: models.TunnelStatusRunning, ListenPort: "20001", CommandLine: "portal://secret@:20001",
		SharedKey: &key, Network: &network, TCPRx: 100,
	}
	if err := db.Create(&portal).Error; err != nil {
		t.Fatal(err)
	}
	subscriptionService := subscription.NewService(db)
	created, err := subscriptionService.Create(subscription.UpsertRequest{Name: "sub", TunnelIDs: []int64{portal.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&portal).Update("tcp_rx", 175).Error; err != nil {
		t.Fatal(err)
	}
	if err := NewService(db).DeleteTunnel(portal.ID); err != nil {
		t.Fatal(err)
	}
	var stored models.PortalSubscription
	if err := db.First(&stored, created.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.TrafficUsed != 75 {
		t.Fatalf("trafficUsed = %d, want 75", stored.TrafficUsed)
	}
	var linkCount int64
	if err := db.Model(&models.PortalSubscriptionTunnel{}).Where("tunnel_id = ?", portal.ID).Count(&linkCount).Error; err != nil {
		t.Fatal(err)
	}
	if linkCount != 0 {
		t.Fatalf("remaining subscription links = %d", linkCount)
	}
}

func TestSubmittedEmptyMetadataClearsStaleInstanceResponse(t *testing.T) {
	staleTags := map[string]string{"region": "old"}
	tunnel := models.Tunnel{}
	applyInstanceState(&tunnel, nowhere.InstanceResult{Meta: &nowhere.Meta{Tags: &staleTags}})
	applySubmittedMetadata(&tunnel, nil, nil)

	if tunnel.Tags != nil || tunnel.Peer != nil {
		t.Fatalf("metadata = tags:%#v peer:%#v, want empty", tunnel.Tags, tunnel.Peer)
	}
}
