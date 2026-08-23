package nowhere

import (
	"NowhereDash/internal/models"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// TunnelConfig is the editable configuration of a Nowhere Portal URL.
type TunnelConfig struct {
	Type       string `json:"type"`
	ListenHost string `json:"listenHost"`
	ListenPort string `json:"listenPort"`
	SharedKey  string `json:"sharedKey"`
	Network    string `json:"network"`
	TLSMode    string `json:"tlsMode"`
	CertPath   string `json:"certPath"`
	KeyPath    string `json:"keyPath"`
	ALPN       string `json:"alpn"`
	Rate       string `json:"rate"`
	Etar       string `json:"etar"`
	Dial       string `json:"dial"`
	Socks      string `json:"socks"`
	Next       string `json:"next"`
	Up         string `json:"up"`
	Down       string `json:"down"`
	PoolSize   string `json:"poolSize"`
	Sni        string `json:"sni"`
	Pin        string `json:"pin"`
	LogLevel   string `json:"logLevel"`
}

func stringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func int64Ptr(value string) *int64 {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil
	}
	return &parsed
}

func valueOr(value *string, fallback string) string {
	if value == nil || *value == "" {
		return fallback
	}
	return *value
}

func isPortalURL(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	return err == nil && strings.EqualFold(parsed.Scheme, string(models.TunnelTypePortal))
}

// PortalConfigURL returns a valid expanded runtime URL from OpenCtrl.
func PortalConfigURL(configLine *string) string {
	if configLine != nil {
		configURL := strings.TrimSpace(*configLine)
		if isPortalURL(configURL) {
			return configURL
		}
	}
	return ""
}

func normalizePortalHost(host string) string {
	value := strings.Trim(strings.TrimSpace(strings.ToLower(host)), "[]")
	switch value {
	case "", "0.0.0.0", "::", "*":
		return ""
	default:
		return value
	}
}

var portalEffectiveQueryKeys = map[string]struct{}{
	"net": {}, "tls": {}, "alpn": {}, "rate": {}, "etar": {},
	"dial": {}, "socks": {}, "next": {}, "up": {}, "down": {},
	"pool": {}, "sni": {}, "pin": {},
}

func withoutAuthorityCredential(value string) string {
	if separator := strings.LastIndex(value, "@"); separator >= 0 {
		return value[separator+1:]
	}
	return value
}

func comparablePortalQueryValue(key, value string) string {
	switch key {
	case "socks", "next":
		// Nowhere deliberately omits credentials from its effective URL.
		return withoutAuthorityCredential(value)
	default:
		return value
	}
}

func portalURLsMatch(commandURL, configURL string) bool {
	command, commandErr := url.Parse(strings.TrimSpace(commandURL))
	config, configErr := url.Parse(strings.TrimSpace(configURL))
	if commandErr != nil || configErr != nil ||
		!strings.EqualFold(command.Scheme, string(models.TunnelTypePortal)) ||
		!strings.EqualFold(config.Scheme, string(models.TunnelTypePortal)) {
		return false
	}
	if command.Port() != config.Port() || normalizePortalHost(command.Hostname()) != normalizePortalHost(config.Hostname()) {
		return false
	}
	// Current Nowhere releases intentionally remove the Portal shared key from
	// the effective URL. Older compatible emitters may still include it.
	if config.User != nil && config.User.Username() != "" &&
		(command.User == nil || command.User.Username() != config.User.Username()) {
		return false
	}

	configQuery := config.Query()
	commandQuery := command.Query()
	nextEnabled := commandQuery.Get("next") != "" && commandQuery.Get("next") != "none"
	for key, commandValues := range commandQuery {
		if _, comparable := portalEffectiveQueryKeys[key]; !comparable {
			continue
		}
		if !nextEnabled {
			switch key {
			case "up", "down", "pool", "sni", "pin":
				continue
			}
		}
		configValues, exists := configQuery[key]
		if !exists || len(commandValues) == 0 || len(configValues) == 0 {
			return false
		}
		if comparablePortalQueryValue(key, commandValues[0]) != comparablePortalQueryValue(key, configValues[0]) {
			return false
		}
	}
	return true
}

// MatchingPortalConfigURL rejects a stale config from a previous command.
func MatchingPortalConfigURL(commandLine string, configLine *string) string {
	configURL := PortalConfigURL(configLine)
	if configURL == "" {
		return ""
	}
	commandURL := strings.TrimSpace(commandLine)
	if commandURL == "" || portalURLsMatch(commandURL, configURL) {
		return configURL
	}
	return ""
}

// EffectivePortalURL resolves the URL used for parsing. The expanded config
// URL wins, while the submitted command URL remains the fallback.
func EffectivePortalURL(commandLine string, configLine *string) string {
	if configURL := MatchingPortalConfigURL(commandLine, configLine); configURL != "" {
		return configURL
	}
	commandURL := strings.TrimSpace(commandLine)
	if isPortalURL(commandURL) {
		return commandURL
	}
	return commandURL
}

// ParseInstanceTunnel parses OpenCtrl's expanded config URL while preserving
// the submitted URL as CommandLine for display and future edits.
func ParseInstanceTunnel(instance InstanceResult) *models.Tunnel {
	effectiveURL := EffectivePortalURL(instance.URL, instance.Config)
	tunnel := ParseTunnelURL(effectiveURL)
	commandTunnel := ParseTunnelURL(strings.TrimSpace(instance.URL))
	if effectiveURL != strings.TrimSpace(instance.URL) && commandTunnel.Type == models.TunnelTypePortal {
		if tunnel.ListenPort == "" {
			tunnel.ListenPort = commandTunnel.ListenPort
		}
		if commandTunnel.SharedKey != nil && *commandTunnel.SharedKey != "" {
			tunnel.SharedKey = commandTunnel.SharedKey
		}
		if tunnel.TLSMode == models.TLS2 {
			if commandTunnel.CertPath != nil && *commandTunnel.CertPath != "" {
				tunnel.CertPath = commandTunnel.CertPath
			}
			if commandTunnel.KeyPath != nil && *commandTunnel.KeyPath != "" {
				tunnel.KeyPath = commandTunnel.KeyPath
			}
		}
		// These values are intentionally absent or credential-stripped in the
		// effective URL, so retain their submitted form for later edits.
		tunnel.LogLevel = commandTunnel.LogLevel
		if commandTunnel.Socks != nil && strings.Contains(*commandTunnel.Socks, "@") {
			tunnel.Socks = commandTunnel.Socks
		}
		if commandTunnel.Next != nil && *commandTunnel.Next != "none" {
			tunnel.Next = commandTunnel.Next
		}
	}
	if commandURL := strings.TrimSpace(instance.URL); commandURL != "" {
		tunnel.CommandLine = commandURL
	}
	if configURL := MatchingPortalConfigURL(instance.URL, instance.Config); configURL != "" {
		tunnel.ConfigLine = &configURL
	} else if instance.Config != nil && strings.TrimSpace(*instance.Config) != "" {
		staleConfig := ""
		tunnel.ConfigLine = &staleConfig
	}
	return tunnel
}

// ApplyInstanceConfig updates only Portal URL fields. Runtime state and
// OpenCtrl metadata remain owned by their dedicated reconciliation paths.
func ApplyInstanceConfig(tunnel *models.Tunnel, instance InstanceResult) {
	parsed := ParseInstanceTunnel(instance)
	if commandURL := strings.TrimSpace(instance.URL); commandURL != "" {
		tunnel.CommandLine = commandURL
	} else if tunnel.CommandLine == "" {
		tunnel.CommandLine = parsed.CommandLine
	}
	if parsed.ConfigLine != nil {
		tunnel.ConfigLine = parsed.ConfigLine
	}
	if parsed.Type != models.TunnelTypePortal {
		return
	}

	tunnel.Type = parsed.Type
	tunnel.ListenHost = parsed.ListenHost
	if parsed.ListenPort != "" {
		tunnel.ListenPort = parsed.ListenPort
	}
	tunnel.TLSMode = parsed.TLSMode
	tunnel.CertPath = parsed.CertPath
	tunnel.KeyPath = parsed.KeyPath
	tunnel.LogLevel = parsed.LogLevel
	if parsed.SharedKey != nil && *parsed.SharedKey != "" {
		tunnel.SharedKey = parsed.SharedKey
	}
	tunnel.Network = parsed.Network
	tunnel.ALPN = parsed.ALPN
	tunnel.Rate = parsed.Rate
	tunnel.Etar = parsed.Etar
	tunnel.Dial = parsed.Dial
	tunnel.Socks = parsed.Socks
	tunnel.Next = parsed.Next
	tunnel.Up = parsed.Up
	tunnel.Down = parsed.Down
	tunnel.PoolSize = parsed.PoolSize
	tunnel.Sni = parsed.Sni
	tunnel.Pin = parsed.Pin
}

// ParseTunnelURL parses a Portal instance URL returned by OpenCtrl.
func ParseTunnelURL(rawURL string) *models.Tunnel {
	now := time.Now()
	tunnel := &models.Tunnel{
		Status:      models.TunnelStatusStopped,
		TLSMode:     models.TLS1,
		LogLevel:    models.LogLevelInfo,
		CommandLine: rawURL,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return tunnel
	}
	if strings.ToLower(parsed.Scheme) != string(models.TunnelTypePortal) {
		return tunnel
	}
	tunnel.Type = models.TunnelTypePortal

	tunnel.ListenHost = parsed.Hostname()
	tunnel.ListenPort = parsed.Port()
	if parsed.User != nil {
		tunnel.SharedKey = stringPtr(parsed.User.Username())
	}

	query := parsed.Query()
	network := query.Get("net")
	if network == "" {
		network = "mix"
	}
	tunnel.Network = &network

	tlsMode := query.Get("tls")
	if tlsMode == "" {
		tlsMode = "1"
	}
	tunnel.TLSMode = models.TLSMode(tlsMode)
	tunnel.CertPath = stringPtr(query.Get("crt"))
	tunnel.KeyPath = stringPtr(query.Get("key"))

	alpn := query.Get("alpn")
	if alpn == "" {
		alpn = "now/1"
	}
	tunnel.ALPN = &alpn

	if rate := int64Ptr(query.Get("rate")); rate != nil {
		tunnel.Rate = rate
	} else {
		zero := int64(0)
		tunnel.Rate = &zero
	}
	if etar := int64Ptr(query.Get("etar")); etar != nil {
		tunnel.Etar = etar
	} else {
		zero := int64(0)
		tunnel.Etar = &zero
	}

	dial := query.Get("dial")
	if dial == "" {
		dial = "auto"
	}
	tunnel.Dial = &dial
	socks := query.Get("socks")
	if socks == "" {
		socks = "none"
	}
	tunnel.Socks = &socks
	next := query.Get("next")
	if next == "" {
		next = "none"
	}
	tunnel.Next = &next

	up := query.Get("up")
	if up == "" {
		up = "udp"
	}
	tunnel.Up = &up
	down := query.Get("down")
	if down == "" {
		down = "udp"
	}
	tunnel.Down = &down
	tunnel.PoolSize = int64Ptr(query.Get("pool"))
	tunnel.Sni = stringPtr(query.Get("sni"))
	tunnel.Pin = stringPtr(query.Get("pin"))

	logLevel := strings.ToLower(query.Get("log"))
	if logLevel == "" {
		logLevel = "info"
	}
	tunnel.LogLevel = models.LogLevel(logLevel)
	return tunnel
}

// ValidatePortalTunnel applies Nowhere's Portal URL rules before OpenCtrl is
// called so operators receive a useful validation error in the dashboard.
func ValidatePortalTunnel(tunnel models.Tunnel) error {
	if tunnel.Type != models.TunnelTypePortal {
		return fmt.Errorf("only portal instances are supported")
	}
	if tunnel.SharedKey == nil || len([]byte(*tunnel.SharedKey)) == 0 || len([]byte(*tunnel.SharedKey)) > 255 {
		return fmt.Errorf("shared key must contain 1 to 255 bytes")
	}
	port, err := strconv.Atoi(tunnel.ListenPort)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("listen port must be between 1 and 65535")
	}

	network := valueOr(tunnel.Network, "mix")
	if network != "mix" && network != "tcp" && network != "udp" {
		return fmt.Errorf("net must be mix, tcp, or udp")
	}
	tlsMode := string(tunnel.TLSMode)
	if tlsMode == "" {
		tlsMode = "1"
	}
	if tlsMode != "1" && tlsMode != "2" {
		return fmt.Errorf("tls must be 1 or 2")
	}
	if tlsMode == "2" {
		if valueOr(tunnel.CertPath, "") == "" || valueOr(tunnel.KeyPath, "") == "" {
			return fmt.Errorf("crt and key are required when tls=2")
		}
	} else if valueOr(tunnel.CertPath, "") != "" || valueOr(tunnel.KeyPath, "") != "" {
		return fmt.Errorf("crt and key may only be set when tls=2")
	}

	alpn := valueOr(tunnel.ALPN, "now/1")
	if len([]byte(alpn)) == 0 || len([]byte(alpn)) > 255 {
		return fmt.Errorf("alpn must contain 1 to 255 bytes")
	}
	if tunnel.Rate != nil && *tunnel.Rate < 0 {
		return fmt.Errorf("rate must be a nonnegative integer")
	}
	if tunnel.Etar != nil && *tunnel.Etar < 0 {
		return fmt.Errorf("etar must be a nonnegative integer")
	}

	dial := valueOr(tunnel.Dial, "auto")
	if dial != "auto" && net.ParseIP(dial) == nil {
		return fmt.Errorf("dial must be auto or a local IP address")
	}
	socks := valueOr(tunnel.Socks, "none")
	next := valueOr(tunnel.Next, "none")
	if socks == "" || next == "" {
		return fmt.Errorf("socks and next cannot be empty; use none to disable them")
	}
	if socks != "none" && next != "none" {
		return fmt.Errorf("socks and next are mutually exclusive")
	}

	if next != "none" {
		for name, transport := range map[string]string{"up": valueOr(tunnel.Up, "udp"), "down": valueOr(tunnel.Down, "udp")} {
			if transport != "tcp" && transport != "udp" {
				return fmt.Errorf("%s must be tcp or udp", name)
			}
		}
		if tunnel.PoolSize != nil && (*tunnel.PoolSize < 0 || *tunnel.PoolSize > 256) {
			return fmt.Errorf("pool must be between 0 and 256")
		}
		if tunnel.Pin != nil && *tunnel.Pin != "" && *tunnel.Pin != "none" {
			if len(*tunnel.Pin) != 64 || strings.ToLower(*tunnel.Pin) != *tunnel.Pin {
				return fmt.Errorf("pin must be a lowercase SHA-256 value")
			}
			if _, err := hex.DecodeString(*tunnel.Pin); err != nil {
				return fmt.Errorf("pin must be a lowercase SHA-256 value")
			}
		}
	}

	logLevel := string(tunnel.LogLevel)
	if logLevel == "" {
		logLevel = "info"
	}
	switch logLevel {
	case "none", "debug", "info", "warn", "error", "event":
	default:
		return fmt.Errorf("invalid log level")
	}
	return nil
}

// BuildTunnelURLs serializes a Portal model into the URL accepted by Nowhere.
func BuildTunnelURLs(tunnel models.Tunnel) string {
	parsed := &url.URL{
		Scheme: "portal",
		Host:   net.JoinHostPort(tunnel.ListenHost, tunnel.ListenPort),
	}
	if tunnel.SharedKey != nil {
		parsed.User = url.User(*tunnel.SharedKey)
	}

	query := url.Values{}
	query.Set("net", valueOr(tunnel.Network, "mix"))
	tlsMode := string(tunnel.TLSMode)
	if tlsMode == "" {
		tlsMode = "1"
	}
	query.Set("tls", tlsMode)
	if tlsMode == "2" {
		query.Set("crt", valueOr(tunnel.CertPath, ""))
		query.Set("key", valueOr(tunnel.KeyPath, ""))
	}
	if alpn := valueOr(tunnel.ALPN, ""); alpn != "" {
		query.Set("alpn", alpn)
	}
	if tunnel.Rate != nil {
		setIntQuery(query, "rate", tunnel.Rate, 0)
	}
	if tunnel.Etar != nil {
		setIntQuery(query, "etar", tunnel.Etar, 0)
	}
	if dial := valueOr(tunnel.Dial, ""); dial != "" {
		query.Set("dial", dial)
	}

	socks := valueOr(tunnel.Socks, "")
	next := valueOr(tunnel.Next, "none")
	if socks != "" {
		query.Set("socks", socks)
	}
	query.Set("next", next)
	if next != "none" {
		query.Set("up", valueOr(tunnel.Up, "udp"))
		query.Set("down", valueOr(tunnel.Down, "udp"))
		if tunnel.PoolSize != nil {
			setIntQuery(query, "pool", tunnel.PoolSize, 0)
		}
		query.Set("sni", valueOr(tunnel.Sni, "none"))
		query.Set("pin", valueOr(tunnel.Pin, "none"))
	}
	logLevel := string(tunnel.LogLevel)
	if logLevel == "" {
		logLevel = "info"
	}
	query.Set("log", logLevel)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func setIntQuery(query url.Values, key string, value *int64, fallback int64) {
	if value == nil {
		query.Set(key, strconv.FormatInt(fallback, 10))
		return
	}
	query.Set(key, strconv.FormatInt(*value, 10))
}

// TunnelToMap creates a complete update map for SSE and refresh reconciliation.
func TunnelToMap(tunnel *models.Tunnel) map[string]interface{} {
	updates := map[string]interface{}{
		"name":            tunnel.Name,
		"status":          tunnel.Status,
		"type":            tunnel.Type,
		"tcp_rx":          tunnel.TCPRx,
		"tcp_tx":          tunnel.TCPTx,
		"udp_rx":          tunnel.UDPRx,
		"udp_tx":          tunnel.UDPTx,
		"tcps":            tunnel.TCPs,
		"udps":            tunnel.UDPs,
		"pool":            tunnel.Pool,
		"ping":            tunnel.Ping,
		"listen_host":     tunnel.ListenHost,
		"listen_port":     tunnel.ListenPort,
		"tls_mode":        tunnel.TLSMode,
		"log_level":       tunnel.LogLevel,
		"command_line":    tunnel.CommandLine,
		"shared_key":      tunnel.SharedKey,
		"cert_path":       tunnel.CertPath,
		"key_path":        tunnel.KeyPath,
		"restart":         tunnel.Restart,
		"last_event_time": tunnel.LastEventTime,
		"updated_at":      time.Now(),
		"network":         tunnel.Network,
		"alpn":            tunnel.ALPN,
		"rate":            tunnel.Rate,
		"etar":            tunnel.Etar,
		"dial":            tunnel.Dial,
		"socks":           tunnel.Socks,
		"next":            tunnel.Next,
		"up":              tunnel.Up,
		"down":            tunnel.Down,
		"pool_size":       tunnel.PoolSize,
		"sni":             tunnel.Sni,
		"pin":             tunnel.Pin,
	}
	if tunnel.ConfigLine != nil {
		updates["config_line"] = tunnel.ConfigLine
	}
	if tunnel.Tags != nil {
		if tagsJSON, err := json.Marshal(tunnel.Tags); err == nil {
			updates["tags"] = string(tagsJSON)
		}
	}
	if tunnel.Peer != nil {
		if peerJSON, err := json.Marshal(tunnel.Peer); err == nil {
			updates["peer"] = string(peerJSON)
		}
	}
	return updates
}

// TunnelConfigFromTunnel exposes the merged Portal values to the details API.
func TunnelConfigFromTunnel(tunnel *models.Tunnel) *TunnelConfig {
	return &TunnelConfig{
		Type:       string(tunnel.Type),
		ListenHost: tunnel.ListenHost,
		ListenPort: tunnel.ListenPort,
		SharedKey:  valueOr(tunnel.SharedKey, ""),
		Network:    valueOr(tunnel.Network, "mix"),
		TLSMode:    string(tunnel.TLSMode),
		CertPath:   valueOr(tunnel.CertPath, ""),
		KeyPath:    valueOr(tunnel.KeyPath, ""),
		ALPN:       valueOr(tunnel.ALPN, "now/1"),
		Rate:       intValue(tunnel.Rate),
		Etar:       intValue(tunnel.Etar),
		Dial:       valueOr(tunnel.Dial, "auto"),
		Socks:      valueOr(tunnel.Socks, "none"),
		Next:       valueOr(tunnel.Next, "none"),
		Up:         valueOr(tunnel.Up, "udp"),
		Down:       valueOr(tunnel.Down, "udp"),
		PoolSize:   intValue(tunnel.PoolSize),
		Sni:        valueOr(tunnel.Sni, "none"),
		Pin:        valueOr(tunnel.Pin, "none"),
		LogLevel:   string(tunnel.LogLevel),
	}
}

// ParseTunnelConfig parses a Portal URL into the details API shape.
func ParseTunnelConfig(rawURL string) *TunnelConfig {
	return TunnelConfigFromTunnel(ParseTunnelURL(rawURL))
}

func intValue(value *int64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatInt(*value, 10)
}

// BuildTunnelConfigURL serializes a parsed Portal configuration.
func (config *TunnelConfig) BuildTunnelConfigURL() string {
	tunnel := models.Tunnel{
		Type:       models.TunnelTypePortal,
		ListenHost: config.ListenHost,
		ListenPort: config.ListenPort,
		TLSMode:    models.TLSMode(config.TLSMode),
		LogLevel:   models.LogLevel(config.LogLevel),
		SharedKey:  stringPtr(config.SharedKey),
		Network:    stringPtr(config.Network),
		CertPath:   stringPtr(config.CertPath),
		KeyPath:    stringPtr(config.KeyPath),
		ALPN:       stringPtr(config.ALPN),
		Rate:       int64Ptr(config.Rate),
		Etar:       int64Ptr(config.Etar),
		Dial:       stringPtr(config.Dial),
		Socks:      stringPtr(config.Socks),
		Next:       stringPtr(config.Next),
		Up:         stringPtr(config.Up),
		Down:       stringPtr(config.Down),
		PoolSize:   int64Ptr(config.PoolSize),
		Sni:        stringPtr(config.Sni),
		Pin:        stringPtr(config.Pin),
	}
	return BuildTunnelURLs(tunnel)
}

// BuildVectorURL returns a ready-to-share Vector URL for connecting to a
// Portal. portalHost is used as the public fallback when the Portal binds a
// wildcard address. The local SOCKS listener defaults to
// 127.0.0.1:1080.
func BuildVectorURL(tunnel models.Tunnel, portalHost, socksListener string) (string, error) {
	if err := ValidatePortalTunnel(tunnel); err != nil {
		return "", err
	}
	listenHost := normalizeHost(tunnel.ListenHost)
	portalHost = normalizeHost(portalHost)
	if listenHost != "" && listenHost != "0.0.0.0" && listenHost != "::" {
		portalHost = listenHost
	}
	if portalHost == "" || portalHost == "0.0.0.0" || portalHost == "::" {
		return "", fmt.Errorf("an externally reachable portal host is required")
	}
	if socksListener == "" {
		socksListener = "127.0.0.1:1080"
	}

	transport := "udp"
	pool := int64(0)
	if valueOr(tunnel.Network, "mix") == "tcp" {
		transport = "tcp"
		pool = 5
	}
	parsed := &url.URL{
		Scheme: "vector",
		Host:   net.JoinHostPort(portalHost, tunnel.ListenPort),
		User:   url.User(valueOr(tunnel.SharedKey, "")),
	}
	query := url.Values{}
	query.Set("up", transport)
	query.Set("down", transport)
	query.Set("pool", strconv.FormatInt(pool, 10))
	query.Set("sni", "none")
	query.Set("pin", "none")
	query.Set("alpn", valueOr(tunnel.ALPN, "now/1"))
	setIntQuery(query, "rate", tunnel.Rate, 0)
	setIntQuery(query, "etar", tunnel.Etar, 0)
	query.Set("socks", socksListener)
	query.Set("log", string(tunnel.LogLevel))
	if tunnel.LogLevel == "" {
		query.Set("log", "info")
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func normalizeHost(host string) string {
	return strings.Trim(strings.TrimSpace(host), "[]")
}
