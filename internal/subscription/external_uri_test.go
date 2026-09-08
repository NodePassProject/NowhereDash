package subscription

import (
	"NowhereDash/internal/models"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestExternalURIValidationMatchesAnywhereSchemes(t *testing.T) {
	sudokuPayload := base64.RawURLEncoding.EncodeToString([]byte(`{"h":"sudoku.example","p":443,"k":"secret"}`))
	valid := []string{
		"nowhere://key@nowhere.example:443?up=tcp&down=udp#Nowhere",
		"vless://123e4567-e89b-12d3-a456-426614174000@vless.example:443?security=tls#VLESS",
		"hysteria2://password@hy2.example:443?sni=hy2.example#Hysteria",
		"hy2://password@hy2-alias.example:443#HysteriaAlias",
		"trojan://password@trojan.example:443?sni=trojan.example#Trojan",
		"anytls://password@anytls.example:443#AnyTLS",
		"ss://YWVzLTEyOC1nY206cGFzc3dvcmQ@ss.example:8388#Shadowsocks",
		"socks5://user:password@socks.example:1080#SOCKS",
		"socks://socks-alias.example:1080#SOCKSAlias",
		"sudoku://" + sudokuPayload,
	}

	nodes, err := validateExternalURIs(valid)
	if err != nil {
		t.Fatalf("validate supported URIs: %v", err)
	}
	if len(nodes) != len(valid) {
		t.Fatalf("node count = %d, want %d", len(nodes), len(valid))
	}
	for index, node := range nodes {
		if node.URI != valid[index] {
			t.Fatalf("node %d URI changed: %q", index, node.URI)
		}
	}
}

func TestExternalURIValidationRejectsUnsupportedAndUnsafeEntries(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   string
	}{
		{name: "unsupported", values: []string{"vmess://payload"}, want: "unsupported URI scheme"},
		{name: "duplicate", values: []string{"socks://proxy.example:1080", "socks://proxy.example:1080"}, want: "duplicates"},
		{name: "newline", values: []string{"trojan://password@example.com:443\nss://payload"}, want: "control characters"},
		{name: "plugin", values: []string{"ss://YWVzLTEyOC1nY206cGFzcw@ss.example:8388?plugin=v2ray-plugin"}, want: "plugins are not supported"},
		{name: "bad UUID", values: []string{"vless://not-a-uuid@example.com:443"}, want: "invalid VLESS UUID"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := validateExternalURIs(test.values)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want text %q", err, test.want)
			}
		})
	}
}

func TestExternalOnlySubscriptionRendersAndUpdates(t *testing.T) {
	db := openSubscriptionTestDB(t, nil)
	service := NewService(db)
	vless := "vless://123e4567-e89b-12d3-a456-426614174000@vless.example:443?security=reality#VLESS"
	trojan := "trojan://password@trojan.example:443?sni=trojan.example#Trojan"

	created, err := service.Create(UpsertRequest{Name: "external", ExternalURIs: []string{vless, trojan}})
	if err != nil {
		t.Fatalf("create external subscription: %v", err)
	}
	if created.PortalCount != 0 || created.ExternalNodeCount != 2 || created.NodeCount != 2 {
		t.Fatalf("created counts = portal:%d external:%d node:%d", created.PortalCount, created.ExternalNodeCount, created.NodeCount)
	}
	if len(created.ExternalURIs) != 2 || created.ExternalURIs[0] != vless || created.ExternalURIs[1] != trojan {
		t.Fatalf("created URIs = %q", created.ExternalURIs)
	}

	rendered, err := service.RenderPublic(created.Token)
	if err != nil {
		t.Fatalf("render external subscription: %v", err)
	}
	if rendered.PortalCount != 0 || rendered.ExternalNodeCount != 2 || rendered.NodeCount != 2 {
		t.Fatalf("rendered counts = %+v", rendered)
	}
	if rendered.Content != vless+"\n"+trojan+"\n" {
		t.Fatalf("rendered content changed or reordered: %q", rendered.Content)
	}

	preview, err := service.Preview(created.ID)
	if err != nil || !preview.Available || preview.NodeCount != 2 {
		t.Fatalf("preview = %+v err=%v", preview, err)
	}
	updated, err := service.Update(created.ID, UpsertRequest{Name: "external", ExternalURIs: []string{trojan}})
	if err != nil || len(updated.ExternalURIs) != 1 || updated.ExternalURIs[0] != trojan {
		t.Fatalf("updated = %+v err=%v", updated, err)
	}
	if err := service.Delete(created.ID); err != nil {
		t.Fatalf("delete external subscription: %v", err)
	}
	if _, err := service.Get(created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get deleted subscription error = %v", err)
	}
}

func TestMixedSubscriptionRendersManagedThenExternalNodes(t *testing.T) {
	db := openSubscriptionTestDB(t, nil)
	portal := seedPortal(t, db, "portal.example", "running", "tcp", 0)
	service := NewService(db)
	external := "trojan://password@trojan.example:443#External"

	created, err := service.Create(UpsertRequest{
		Name: "mixed", TunnelIDs: []int64{portal.ID}, ExternalURIs: []string{external},
	})
	if err != nil {
		t.Fatalf("create mixed subscription: %v", err)
	}
	rendered, err := service.RenderPublic(created.Token)
	if err != nil {
		t.Fatalf("render mixed subscription: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(rendered.Content, "\n"), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "nowhere://") || lines[1] != external {
		t.Fatalf("mixed content order = %q", rendered.Content)
	}
	if rendered.PortalCount != 1 || rendered.ExternalNodeCount != 1 || rendered.NodeCount != 2 {
		t.Fatalf("rendered counts = %+v", rendered)
	}
}

func TestMixedSubscriptionRespectsUnifiedNodeOrder(t *testing.T) {
	db := openSubscriptionTestDB(t, nil)
	portal := seedPortal(t, db, "portal.example", "running", "tcp", 0)
	service := NewService(db)
	external := "trojan://password@trojan.example:443#External"

	created, err := service.Create(UpsertRequest{
		Name: "reordered", TunnelIDs: []int64{portal.ID}, ExternalURIs: []string{external},
	})
	if err != nil {
		t.Fatalf("create subscription before reorder: %v", err)
	}
	_, err = service.Update(created.ID, UpsertRequest{
		Name: "reordered", TunnelIDs: []int64{portal.ID}, ExternalURIs: []string{external},
		NodeOrder: []NodeOrderItem{
			{Source: "url", URI: external},
			{Source: "portal", TunnelID: portal.ID},
		},
	})
	if err != nil {
		t.Fatalf("save reordered subscription: %v", err)
	}
	reloaded, err := service.Get(created.ID)
	if err != nil {
		t.Fatalf("reload reordered subscription: %v", err)
	}
	if len(reloaded.NodeOrder) != 2 || reloaded.NodeOrder[0].Source != "url" || reloaded.NodeOrder[1].TunnelID != portal.ID {
		t.Fatalf("stored node order = %#v", reloaded.NodeOrder)
	}

	rendered, err := service.RenderPublic(created.Token)
	if err != nil {
		t.Fatalf("render reordered subscription: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(rendered.Content, "\n"), "\n")
	if len(lines) != 2 || lines[0] != external || !strings.HasPrefix(lines[1], "nowhere://") {
		t.Fatalf("reordered mixed content = %q", rendered.Content)
	}
}

func TestManagedNodeNameOverrideChangesOnlySubscriptionFragment(t *testing.T) {
	db := openSubscriptionTestDB(t, nil)
	portal := seedPortal(t, db, "portal.example", "running", "tcp", 0)
	service := NewService(db)

	created, err := service.Create(UpsertRequest{
		Name: "renamed portal", TunnelIDs: []int64{portal.ID},
		TunnelNames: map[int64]string{portal.ID: "Singapore Edge"},
	})
	if err != nil {
		t.Fatalf("create subscription with managed node name: %v", err)
	}
	if created.TunnelNames[portal.ID] != "Singapore Edge" {
		t.Fatalf("tunnelNames = %#v", created.TunnelNames)
	}

	rendered, err := service.RenderPublic(created.Token)
	if err != nil {
		t.Fatalf("render renamed managed node: %v", err)
	}
	if !strings.Contains(rendered.Content, "#Singapore%20Edge") {
		t.Fatalf("managed node fragment was not renamed: %q", rendered.Content)
	}
	if strings.Contains(rendered.Content, "#"+portal.Name) {
		t.Fatalf("managed node still uses Portal name: %q", rendered.Content)
	}

	var stored models.Tunnel
	if err := db.First(&stored, portal.ID).Error; err != nil {
		t.Fatalf("load Portal: %v", err)
	}
	if stored.Name != portal.Name {
		t.Fatalf("Portal name changed to %q", stored.Name)
	}
}
