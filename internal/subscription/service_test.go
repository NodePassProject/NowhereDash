package subscription

import (
	"NowhereDash/internal/models"
	"NowhereDash/internal/nowhere"
	"bytes"
	"errors"
	"fmt"
	standardlog "log"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openSubscriptionTestDB(t *testing.T, configuredLogger logger.Interface) *gorm.DB {
	t.Helper()
	configuration := &gorm.Config{}
	if configuredLogger != nil {
		configuration.Logger = configuredLogger
	}
	dsn := fmt.Sprintf("file:subscription-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), configuration)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql database: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(
		&models.Endpoint{}, &models.Tunnel{},
		&models.PortalSubscription{}, &models.PortalSubscriptionTunnel{},
		&models.TunnelOperationLog{},
	); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	return db
}

func seedPortal(t *testing.T, db *gorm.DB, host string, status models.TunnelStatus, network string, traffic int64) models.Tunnel {
	t.Helper()
	endpoint := models.Endpoint{
		Name: "master-" + host, URL: "master://api.example:10101/" + host, Hostname: host,
		APIPath: "/api/v2", APIKey: "test-key",
	}
	if err := db.Create(&endpoint).Error; err != nil {
		t.Fatalf("create endpoint: %v", err)
	}
	sharedKey, alpn := "key:/@", "now/1 test"
	portal := models.Tunnel{
		Name: "SG node 01", EndpointID: endpoint.ID, Type: models.TunnelTypePortal,
		Status: status, ListenHost: "*", ListenPort: "20001", TLSMode: models.TLS1,
		CommandLine: "portal://key@:20001", SharedKey: &sharedKey, Network: &network,
		ALPN: &alpn, LogLevel: models.LogLevelInfo, TCPRx: traffic,
	}
	if err := db.Create(&portal).Error; err != nil {
		t.Fatalf("create Portal: %v", err)
	}
	return portal
}

func int64Pointer(value int64) *int64 { return &value }

func attachInstance(t *testing.T, db *gorm.DB, tunnel *models.Tunnel, instanceID string) {
	t.Helper()
	tunnel.InstanceID = &instanceID
	if err := db.Model(tunnel).Update("instance_id", instanceID).Error; err != nil {
		t.Fatalf("attach instance: %v", err)
	}
}

func TestCreatePreservesPreferencesAndEstablishesTrafficBaseline(t *testing.T) {
	db := openSubscriptionTestDB(t, nil)
	portal := seedPortal(t, db, "portal.example", models.TunnelStatusRunning, "mix", 100)
	service := NewService(db)

	created, err := service.Create(UpsertRequest{
		Name: "private", TrafficLimit: int64Pointer(40),
		Preferences: &Preferences{ExpandCarrierCombos: false, UpCarrier: "udp", DownCarrier: "tcp"},
		TunnelIDs:   []int64{portal.ID},
	})
	if err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	if created.Preferences.ExpandCarrierCombos {
		t.Fatal("expandCarrierCombos = true, want explicit false")
	}
	if created.TrafficUsed != 0 {
		t.Fatalf("trafficUsed = %d, want 0 at association baseline", created.TrafficUsed)
	}
	var link models.PortalSubscriptionTunnel
	if err := db.Where("subscription_id = ?", created.ID).First(&link).Error; err != nil {
		t.Fatalf("load link: %v", err)
	}
	if link.BaselineBytes != 100 || link.LastObservedBytes != 100 || link.AccountedBytes != 0 {
		t.Fatalf("unexpected accounting cursor: %+v", link)
	}
}

func TestTrafficAccountingUsesNonnegativeDeltasAcrossCounterReset(t *testing.T) {
	db := openSubscriptionTestDB(t, nil)
	portal := seedPortal(t, db, "portal.example", models.TunnelStatusRunning, "tcp", 100)
	service := NewService(db)
	created, err := service.Create(UpsertRequest{Name: "traffic", TunnelIDs: []int64{portal.ID}, TrafficLimit: int64Pointer(40)})
	if err != nil {
		t.Fatalf("create subscription: %v", err)
	}

	if err := db.Model(&portal).Update("tcp_rx", 130).Error; err != nil {
		t.Fatal(err)
	}
	current, err := service.Get(created.ID)
	if err != nil || current.TrafficUsed != 30 {
		t.Fatalf("after increment: response=%+v err=%v", current, err)
	}
	if err := db.Model(&portal).Update("tcp_rx", 5).Error; err != nil {
		t.Fatal(err)
	}
	current, err = service.Get(created.ID)
	if err != nil || current.TrafficUsed != 30 {
		t.Fatalf("after reset: response=%+v err=%v", current, err)
	}
	if err := db.Model(&portal).Update("tcp_rx", 15).Error; err != nil {
		t.Fatal(err)
	}
	current, err = service.Get(created.ID)
	if err != nil || current.TrafficUsed != 40 || !current.OverLimit {
		t.Fatalf("after new epoch increment: response=%+v err=%v", current, err)
	}
	if _, err := service.RenderPublic(created.Token); !errors.Is(err, ErrEntitlementUnavailable) {
		t.Fatalf("RenderPublic over limit error = %v", err)
	}
}

func TestOverLimitSubscriptionStopsAllRunningPortals(t *testing.T) {
	db := openSubscriptionTestDB(t, nil)
	portalA := seedPortal(t, db, "a.example", models.TunnelStatusRunning, "tcp", 100)
	portalB := seedPortal(t, db, "b.example", models.TunnelStatusRunning, "tcp", 10)
	attachInstance(t, db, &portalA, "portal-a")
	attachInstance(t, db, &portalB, "portal-b")

	service := NewService(db)
	var stopped []string
	service.controlInstance = func(endpointID int64, instanceID, action string) (nowhere.InstanceResult, error) {
		if action != "stop" {
			t.Fatalf("action = %q, want stop", action)
		}
		stopped = append(stopped, instanceID)
		return nowhere.InstanceResult{ID: instanceID, Status: string(models.TunnelStatusStopped)}, nil
	}
	created, err := service.Create(UpsertRequest{
		Name: "traffic", TunnelIDs: []int64{portalA.ID, portalB.ID}, TrafficLimit: int64Pointer(40),
	})
	if err != nil {
		t.Fatalf("create subscription: %v", err)
	}

	if err := db.Model(&portalA).Update("tcp_rx", 150).Error; err != nil {
		t.Fatal(err)
	}
	current, err := service.Get(created.ID)
	if err != nil {
		t.Fatalf("get subscription: %v", err)
	}
	if !current.OverLimit || current.TrafficUsed != 50 {
		t.Fatalf("subscription = %+v, want over limit with 50 bytes used", current)
	}
	if len(stopped) != 2 {
		t.Fatalf("stopped instances = %v, want both associated running portals", stopped)
	}
	got := map[string]bool{stopped[0]: true, stopped[1]: true}
	if !got["portal-a"] || !got["portal-b"] {
		t.Fatalf("stopped instances = %v", stopped)
	}
	var stoppedCount int64
	if err := db.Model(&models.Tunnel{}).
		Where("id IN ? AND status = ?", []int64{portalA.ID, portalB.ID}, models.TunnelStatusStopped).
		Count(&stoppedCount).Error; err != nil {
		t.Fatal(err)
	}
	if stoppedCount != 2 {
		t.Fatalf("stopped tunnel count = %d, want 2", stoppedCount)
	}
}

func TestRenderPublicProducesNowhereLinesAndPhysicalPortalCount(t *testing.T) {
	db := openSubscriptionTestDB(t, nil)
	portal := seedPortal(t, db, "[2001:db8::10]", models.TunnelStatusRunning, "mix", 0)
	service := NewService(db)
	created, err := service.Create(UpsertRequest{
		Name: "mobile", ProfileTitle: "Nowhere SG", TunnelIDs: []int64{portal.ID},
		Preferences: &Preferences{ExpandCarrierCombos: true, UpCarrier: "tcp", DownCarrier: "tcp", IncludeIPv6: true},
	})
	if err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	rendered, err := service.RenderPublic(created.Token)
	if err != nil {
		t.Fatalf("render public: %v", err)
	}
	if rendered.PortalCount != 1 {
		t.Fatalf("portalCount = %d, want 1 physical Portal", rendered.PortalCount)
	}
	lines := strings.Split(strings.TrimSuffix(rendered.Content, "\n"), "\n")
	if len(lines) != 8 {
		t.Fatalf("credential line count = %d, want 8 base + IPv6 variants: %q", len(lines), rendered.Content)
	}
	if strings.Count(rendered.Content, "pool=5") != 2 {
		t.Fatalf("pool=5 count is not limited to TCP/TCP: %q", rendered.Content)
	}
	for _, expected := range []string{
		"nowhere://key%3A%2F%40@api.example:20001",
		"nowhere://key%3A%2F%40@[2001:db8::10]:20001",
		"alpn=now%2F1%20test",
		"#SG%20node%2001",
		"%7C%20v6",
	} {
		if !strings.Contains(rendered.Content, expected) {
			t.Fatalf("content does not contain %q: %q", expected, rendered.Content)
		}
	}
	if got := rendered.Headers["profile-title"]; got != "base64:Tm93aGVyZSBTRw==" {
		t.Fatalf("profile-title = %q", got)
	}
	if !strings.Contains(rendered.Headers["subscription-userinfo"], "download=0; total=-1") {
		t.Fatalf("subscription-userinfo = %q", rendered.Headers["subscription-userinfo"])
	}
}

func TestPortalRenderingRespectsCarrierIPv6AndALPNPreferences(t *testing.T) {
	key, network := "secret", "tcp"
	portal := models.Tunnel{
		Name: "portal", Type: models.TunnelTypePortal, Status: models.TunnelStatusRunning,
		ListenHost: "2001:db8::1", ListenPort: "20001", SharedKey: &key, Network: &network,
		Endpoint: models.Endpoint{Hostname: "portal.example", URL: "master://other.example:10101"},
	}
	if lines := renderPortal(&portal, Preferences{UpCarrier: "udp", DownCarrier: "udp", IncludeIPv6: true}); len(lines) != 0 {
		t.Fatalf("incompatible pure TCP preferences rendered %q", lines)
	}
	if lines := renderPortal(&portal, Preferences{UpCarrier: "tcp", DownCarrier: "tcp"}); len(lines) != 0 {
		t.Fatalf("IPv6 rendered while includeIpv6=false: %q", lines)
	}
	lines := renderPortal(&portal, Preferences{UpCarrier: "tcp", DownCarrier: "tcp", IncludeIPv6: true})
	if len(lines) != 1 || strings.Contains(lines[0], "alpn=") || strings.Contains(lines[0], "portal.example") {
		t.Fatalf("empty ALPN handling = %q", lines)
	}
	portal.ListenHost = "*"
	portal.Endpoint = models.Endpoint{Hostname: "*", URL: "master://fallback.example:10101"}
	if host := portalHost(&portal, false); host != "fallback.example" {
		t.Fatalf("wildcard fallback host = %q", host)
	}
	portal.Endpoint.Hostname = "[2001:db8::10]"
	if host := portalHost(&portal, false); host != "fallback.example" {
		t.Fatalf("IPv6-disabled fallback host = %q", host)
	}
	portal.ListenHost = "0:0:0:0:0:0:0:0"
	portal.Endpoint.Hostname = "::0"
	portal.Endpoint.URL = "master://public.example:10101"
	if host := portalHost(&portal, true); host != "public.example" {
		t.Fatalf("expanded unspecified IPv6 fallback host = %q", host)
	}
}

func TestTokenDatabaseOperationsDoNotLogSecrets(t *testing.T) {
	var output bytes.Buffer
	configuredLogger := logger.New(standardlog.New(&output, "", 0), logger.Config{LogLevel: logger.Info})
	db := openSubscriptionTestDB(t, configuredLogger)
	service := NewService(db)
	output.Reset()

	created, err := service.Create(UpsertRequest{Name: "secret"})
	if err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	if strings.Contains(output.String(), created.Token) {
		t.Fatalf("create SQL log leaked token: %s", output.String())
	}
	output.Reset()
	rotated, err := service.RotateToken(created.ID)
	if err != nil {
		t.Fatalf("rotate token: %v", err)
	}
	if strings.Contains(output.String(), created.Token) || strings.Contains(output.String(), rotated.Token) {
		t.Fatalf("rotate SQL log leaked token: %s", output.String())
	}
	output.Reset()
	missing := strings.Repeat("A", 43)
	if _, err := service.RenderPublic(missing); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing token error = %v", err)
	}
	if strings.Contains(output.String(), missing) {
		t.Fatalf("lookup SQL log leaked token: %s", output.String())
	}
}

func TestExpiredSubscriptionIsUnavailable(t *testing.T) {
	db := openSubscriptionTestDB(t, nil)
	portal := seedPortal(t, db, "portal.example", models.TunnelStatusRunning, "udp", 0)
	past := time.Now().Add(-time.Second)
	service := NewService(db)
	created, err := service.Create(UpsertRequest{Name: "expired", ExpiresAt: &past, TunnelIDs: []int64{portal.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RenderPublic(created.Token); !errors.Is(err, ErrEntitlementUnavailable) {
		t.Fatalf("expired subscription error = %v", err)
	}
}

func TestExpiredSubscriptionStopsRunningPortals(t *testing.T) {
	db := openSubscriptionTestDB(t, nil)
	portal := seedPortal(t, db, "portal.example", models.TunnelStatusRunning, "udp", 0)
	attachInstance(t, db, &portal, "expired-portal")
	past := time.Now().Add(-time.Second)
	service := NewService(db)
	var stopped []string
	service.controlInstance = func(endpointID int64, instanceID, action string) (nowhere.InstanceResult, error) {
		stopped = append(stopped, instanceID)
		return nowhere.InstanceResult{ID: instanceID, Status: string(models.TunnelStatusStopped)}, nil
	}

	created, err := service.Create(UpsertRequest{Name: "expired", ExpiresAt: &past, TunnelIDs: []int64{portal.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if len(stopped) != 1 || stopped[0] != "expired-portal" {
		t.Fatalf("stopped instances = %v, want expired-portal", stopped)
	}
	if _, err := service.RenderPublic(created.Token); !errors.Is(err, ErrEntitlementUnavailable) {
		t.Fatalf("expired subscription error = %v", err)
	}
}

func TestSubscriptionCRUDPreviewRotateResetAndDelete(t *testing.T) {
	db := openSubscriptionTestDB(t, nil)
	portal := seedPortal(t, db, "portal.example", models.TunnelStatusRunning, "udp", 0)
	service := NewService(db)
	created, err := service.Create(UpsertRequest{Name: "original", TunnelIDs: []int64{portal.ID}})
	if err != nil {
		t.Fatal(err)
	}
	listed, err := service.List()
	if err != nil || listed.Total != 1 || len(listed.Data) != 1 {
		t.Fatalf("list = %+v err=%v", listed, err)
	}
	updated, err := service.Update(created.ID, UpsertRequest{
		Name: "updated", ProfileTitle: "Updated profile", TunnelIDs: []int64{portal.ID},
	})
	if err != nil || updated.Name != "updated" || updated.ProfileTitle != "Updated profile" {
		t.Fatalf("update = %+v err=%v", updated, err)
	}
	preview, err := service.Preview(created.ID)
	if err != nil || !preview.Available || preview.PortalCount != 1 || preview.Content == "" {
		t.Fatalf("preview = %+v err=%v", preview, err)
	}
	rotated, err := service.RotateToken(created.ID)
	if err != nil || rotated.Token == created.Token {
		t.Fatalf("rotate = %+v err=%v", rotated, err)
	}
	if _, err := service.RenderPublic(created.Token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("old token error = %v", err)
	}
	if _, err := service.RenderPublic(rotated.Token); err != nil {
		t.Fatalf("new token error = %v", err)
	}
	if err := db.Model(&portal).Update("tcp_rx", 10).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(created.ID); err != nil {
		t.Fatal(err)
	}
	reset, err := service.ResetTraffic(created.ID)
	if err != nil || reset.TrafficUsed != 0 || reset.OverLimit {
		t.Fatalf("reset = %+v err=%v", reset, err)
	}
	if err := service.Delete(created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get deleted error = %v", err)
	}
}
