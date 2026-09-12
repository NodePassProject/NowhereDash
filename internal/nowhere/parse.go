package nowhere

import (
	"NowhereDash/internal/models"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// TunnelConfig is the editable configuration of a Nowhere Portal URL.
type TunnelConfig struct {
	Type       string  `json:"type"`
	ListenHost string  `json:"listenHost"`
	ListenPort string  `json:"listenPort"`
	TCPPort    *string `json:"tcpPort"`
	UDPPort    *string `json:"udpPort"`
	TCPFamily  string  `json:"tcpFamily"`
	UDPFamily  string  `json:"udpFamily"`
	Morph      string  `json:"morph"`
	SharedKey  string  `json:"sharedKey"`
	Network    string  `json:"network"`
	TLSMode    string  `json:"tlsMode"`
	CertPath   string  `json:"certPath"`
	KeyPath    string  `json:"keyPath"`
	ALPN       string  `json:"alpn"`
	Rate       string  `json:"rate"`
	Etar       string  `json:"etar"`
	Dial       string  `json:"dial"`
	Socks      string  `json:"socks"`
	Next       string  `json:"next"`
	Up         string  `json:"up"`
	Down       string  `json:"down"`
	Mux        string  `json:"mux"`
	Sni        string  `json:"sni"`
	Pin        string  `json:"pin"`
	LogLevel   string  `json:"logLevel"`
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

var portalQueryDefaults = map[string]string{
	"tls": "1", "morph": "0", "rate": "0", "etar": "0",
	"dial": "auto", "socks": "none", "next": "none",
}

func withoutAuthorityCredential(value string) string {
	if separator := strings.LastIndex(value, "@"); separator >= 0 {
		return value[separator+1:]
	}
	return value
}

func comparablePortalQuery(parsed *url.URL) (url.Values, error) {
	query, err := portalQuery(parsed)
	if err != nil {
		return nil, err
	}
	values := url.Values{}
	for key, fallback := range portalQueryDefaults {
		value := query.Get(key)
		if !query.Has(key) {
			value = fallback
		}
		values.Set(key, value)
	}
	if values.Get("socks") != "none" {
		endpoint, err := parseSocksEndpoint(query.Get("socks"), false)
		if err != nil {
			return nil, err
		}
		values.Set("socks", endpoint)
	}
	if next := values.Get("next"); next != "none" {
		parsedNext, err := url.Parse("vector://" + withoutAuthorityCredential(next))
		if err != nil {
			return nil, err
		}
		endpoint, err := parseServiceEndpoint(parsedNext, false)
		if err != nil {
			return nil, err
		}
		values.Set("next", endpoint.Canonical())
		for key, fallback := range map[string]string{"up": endpoint.DefaultCarrier(), "down": endpoint.DefaultCarrier(), "mux": "0", "sni": "none", "pin": "none"} {
			value := query.Get(key)
			if value == "" {
				value = fallback
			}
			values.Set(key, value)
		}
		if values.Get("up") == "udp" && values.Get("down") == "udp" {
			values.Set("mux", "0")
		}
	}
	for _, key := range []string{"rate", "etar"} {
		if number, err := strconv.ParseInt(values.Get(key), 10, 32); err == nil {
			values.Set(key, strconv.FormatInt(number, 10))
		}
	}
	if ip, err := netip.ParseAddr(values.Get("dial")); err == nil {
		values.Set("dial", ip.String())
	}
	return values, nil
}

func portalURLsMatch(commandURL, configURL string) bool {
	command, commandErr := url.Parse(strings.TrimSpace(commandURL))
	config, configErr := url.Parse(strings.TrimSpace(configURL))
	if commandErr != nil || configErr != nil ||
		!strings.EqualFold(command.Scheme, string(models.TunnelTypePortal)) ||
		!strings.EqualFold(config.Scheme, string(models.TunnelTypePortal)) {
		return false
	}
	commandEndpoint, commandErr := parseServiceEndpoint(command, true)
	configEndpoint, configErr := parseServiceEndpoint(config, true)
	if commandErr != nil || configErr != nil || commandEndpoint.Canonical() != configEndpoint.Canonical() {
		return false
	}
	// Current Nowhere releases intentionally remove the Portal shared key from
	// the effective URL. Older compatible emitters may still include it.
	if config.User != nil && config.User.Username() != "" &&
		(command.User == nil || command.User.Username() != config.User.Username()) {
		return false
	}

	configQuery, configErr := comparablePortalQuery(config)
	commandQuery, commandErr := comparablePortalQuery(command)
	if configErr != nil || commandErr != nil {
		return false
	}
	for key := range commandQuery {
		if commandQuery.Get(key) != configQuery.Get(key) {
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
	tunnel.CommandLine = strings.TrimSpace(instance.URL)
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
	if strings.TrimSpace(instance.URL) == "" && PortalConfigURL(instance.Config) == "" {
		return
	}
	if strings.TrimSpace(instance.URL) == "" {
		instance.URL = tunnel.CommandLine
	}
	parsed := ParseInstanceTunnel(instance)
	if parsed.Type != models.TunnelTypePortal {
		return
	}
	if commandURL := strings.TrimSpace(instance.URL); commandURL != "" {
		tunnel.CommandLine = commandURL
	} else if tunnel.CommandLine == "" {
		tunnel.CommandLine = parsed.CommandLine
	}
	if parsed.ConfigLine != nil {
		tunnel.ConfigLine = parsed.ConfigLine
	} else if tunnel.ConfigLine != nil && MatchingPortalConfigURL(tunnel.CommandLine, tunnel.ConfigLine) == "" {
		empty := ""
		tunnel.ConfigLine = &empty
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
	tunnel.TCPPort, tunnel.UDPPort = parsed.TCPPort, parsed.UDPPort
	tunnel.TCPFamily, tunnel.UDPFamily = parsed.TCPFamily, parsed.UDPFamily
	tunnel.Morph = parsed.Morph
	tunnel.ALPN = parsed.ALPN
	tunnel.Rate = parsed.Rate
	tunnel.Etar = parsed.Etar
	tunnel.Dial = parsed.Dial
	tunnel.Socks = parsed.Socks
	tunnel.Next = parsed.Next
	tunnel.Up = parsed.Up
	tunnel.Down = parsed.Down
	tunnel.Mux = parsed.Mux
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

	endpoint, endpointErr := parseServiceEndpoint(parsed, true)
	if endpointErr != nil {
		tunnel.Type = ""
		return tunnel
	}
	applyEndpoint(tunnel, endpoint)
	if parsed.User != nil {
		tunnel.SharedKey = stringPtr(parsed.User.Username())
	}

	query, err := portalQuery(parsed)
	if err != nil {
		tunnel.Type = ""
		return tunnel
	}
	morph := "0"
	if query.Has("morph") {
		morph = query.Get("morph")
	}
	tunnel.Morph = &morph

	tlsMode := query.Get("tls")
	if tlsMode == "" {
		tlsMode = "1"
	}
	tunnel.TLSMode = models.TLSMode(tlsMode)
	tunnel.CertPath = stringPtr(query.Get("crt"))
	tunnel.KeyPath = stringPtr(query.Get("key"))

	alpn := "nw2"
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
		up = nextDefaultCarrier(tunnel.Next)
	}
	tunnel.Up = &up
	down := query.Get("down")
	if down == "" {
		down = nextDefaultCarrier(tunnel.Next)
	}
	tunnel.Down = &down
	mux := query.Get("mux")
	if mux == "" {
		mux = "0"
	}
	if next != "none" && up == "udp" && down == "udp" && mux == "1" {
		mux = "0"
	}
	tunnel.Mux = stringPtr(mux)
	tunnel.Sni = stringPtr(query.Get("sni"))
	tunnel.Pin = stringPtr(query.Get("pin"))

	logLevel := query.Get("log")
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
	if err := EndpointFromTunnel(tunnel).Validate(true); err != nil {
		return err
	}
	if tunnel.Morph != nil && *tunnel.Morph != "0" && *tunnel.Morph != "1" {
		return fmt.Errorf("morph must be 0 or 1")
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

	if tunnel.Rate != nil && (*tunnel.Rate < 0 || *tunnel.Rate > math.MaxInt32) {
		return fmt.Errorf("rate must be an integer between 0 and 2147483647")
	}
	if tunnel.Etar != nil && (*tunnel.Etar < 0 || *tunnel.Etar > math.MaxInt32) {
		return fmt.Errorf("etar must be an integer between 0 and 2147483647")
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
	if socks != "none" {
		if _, err := parseSocksEndpoint(socks, false); err != nil {
			return err
		}
	}

	if next != "none" {
		endpoint, err := parseNextEndpoint(next)
		if err != nil {
			return err
		}
		for name, transport := range map[string]string{"up": valueOr(tunnel.Up, endpoint.DefaultCarrier()), "down": valueOr(tunnel.Down, endpoint.DefaultCarrier())} {
			if transport != "mix" && transport != "tcp" && transport != "udp" {
				return fmt.Errorf("%s must be mix, tcp, or udp", name)
			}
			if (transport != "udp" && endpoint.TCPPort == "") || (transport != "tcp" && endpoint.UDPPort == "") {
				return fmt.Errorf("%s selects a carrier missing from the next endpoint", name)
			}
		}
		mux := valueOr(tunnel.Mux, "0")
		if mux != "0" && mux != "1" {
			return fmt.Errorf("mux must be 0 or 1")
		}
		if err := validateSNI(valueOr(tunnel.Sni, "none")); err != nil {
			return err
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
	parsed, err := url.Parse("portal://" + EndpointFromTunnel(tunnel).Canonical())
	if err != nil {
		return ""
	}
	if tunnel.SharedKey != nil {
		parsed.User = url.User(*tunnel.SharedKey)
	}

	query := url.Values{}
	query.Set("morph", valueOr(tunnel.Morph, "0"))
	tlsMode := string(tunnel.TLSMode)
	if tlsMode == "" {
		tlsMode = "1"
	}
	query.Set("tls", tlsMode)
	if tlsMode == "2" {
		query.Set("crt", valueOr(tunnel.CertPath, ""))
		query.Set("key", valueOr(tunnel.KeyPath, ""))
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
		up := valueOr(tunnel.Up, nextDefaultCarrier(tunnel.Next))
		down := valueOr(tunnel.Down, nextDefaultCarrier(tunnel.Next))
		query.Set("up", up)
		query.Set("down", down)
		mux := valueOr(tunnel.Mux, "0")
		if up == "udp" && down == "udp" {
			mux = "0"
		}
		query.Set("mux", mux)
		query.Set("sni", valueOr(tunnel.Sni, "none"))
		query.Set("pin", valueOr(tunnel.Pin, "none"))
	}
	logLevel := string(tunnel.LogLevel)
	if logLevel == "" {
		logLevel = "info"
	}
	query.Set("log", logLevel)
	parsed.RawQuery = encodeNativeQuery(query)
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
		"tcp_port":        tunnel.TCPPort,
		"udp_port":        tunnel.UDPPort,
		"tcp_family":      tunnel.TCPFamily,
		"udp_family":      tunnel.UDPFamily,
		"morph":           tunnel.Morph,
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
		"mux":             tunnel.Mux,
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
		TCPPort:    tunnel.TCPPort, UDPPort: tunnel.UDPPort,
		TCPFamily: tunnel.TCPFamily, UDPFamily: tunnel.UDPFamily,
		Morph:     valueOr(tunnel.Morph, "0"),
		SharedKey: valueOr(tunnel.SharedKey, ""),
		Network:   valueOr(tunnel.Network, "mix"),
		TLSMode:   string(tunnel.TLSMode),
		CertPath:  valueOr(tunnel.CertPath, ""),
		KeyPath:   valueOr(tunnel.KeyPath, ""),
		ALPN:      "nw2",
		Rate:      intValue(tunnel.Rate),
		Etar:      intValue(tunnel.Etar),
		Dial:      valueOr(tunnel.Dial, "auto"),
		Socks:     valueOr(tunnel.Socks, "none"),
		Next:      valueOr(tunnel.Next, "none"),
		Up:        valueOr(tunnel.Up, nextDefaultCarrier(tunnel.Next)),
		Down:      valueOr(tunnel.Down, nextDefaultCarrier(tunnel.Next)),
		Mux:       valueOr(tunnel.Mux, "0"),
		Sni:       valueOr(tunnel.Sni, "none"),
		Pin:       valueOr(tunnel.Pin, "none"),
		LogLevel:  string(tunnel.LogLevel),
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
		TCPPort:    config.TCPPort, UDPPort: config.UDPPort,
		TCPFamily: config.TCPFamily, UDPFamily: config.UDPFamily,
		Morph:     stringPtr(config.Morph),
		TLSMode:   models.TLSMode(config.TLSMode),
		LogLevel:  models.LogLevel(config.LogLevel),
		SharedKey: stringPtr(config.SharedKey),
		Network:   stringPtr(config.Network),
		CertPath:  stringPtr(config.CertPath),
		KeyPath:   stringPtr(config.KeyPath),
		ALPN:      stringPtr(config.ALPN),
		Rate:      int64Ptr(config.Rate),
		Etar:      int64Ptr(config.Etar),
		Dial:      stringPtr(config.Dial),
		Socks:     stringPtr(config.Socks),
		Next:      stringPtr(config.Next),
		Up:        stringPtr(config.Up),
		Down:      stringPtr(config.Down),
		Mux:       stringPtr(config.Mux),
		Sni:       stringPtr(config.Sni),
		Pin:       stringPtr(config.Pin),
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
	if listenHost != "" && listenHost != "*" && listenHost != "0.0.0.0" && listenHost != "::" {
		portalHost = listenHost
	}
	if portalHost == "" || portalHost == "*" || portalHost == "0.0.0.0" || portalHost == "::" {
		return "", fmt.Errorf("an externally reachable portal host is required")
	}
	if socksListener == "" {
		socksListener = "127.0.0.1:1080"
	}
	if _, err := parseSocksEndpoint(socksListener, true); err != nil {
		return "", err
	}

	endpoint := EndpointFromTunnel(tunnel)
	endpoint.Host = portalHost
	if err := endpoint.Validate(false); err != nil {
		return "", err
	}
	transport := endpoint.DefaultCarrier()
	parsed, err := url.Parse("vector://" + endpoint.Canonical())
	if err != nil {
		return "", err
	}
	parsed.User = url.User(valueOr(tunnel.SharedKey, ""))
	query := url.Values{}
	query.Set("up", transport)
	query.Set("down", transport)
	query.Set("mux", "0")
	query.Set("morph", valueOr(tunnel.Morph, "0"))
	query.Set("sni", "none")
	query.Set("pin", "none")
	setIntQuery(query, "rate", tunnel.Rate, 0)
	setIntQuery(query, "etar", tunnel.Etar, 0)
	query.Set("socks", socksListener)
	query.Set("log", string(tunnel.LogLevel))
	if tunnel.LogLevel == "" {
		query.Set("log", "info")
	}
	parsed.RawQuery = encodeNativeQuery(query)
	return parsed.String(), nil
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		if ip, err := netip.ParseAddr(host[1 : len(host)-1]); err == nil && ip.Is6() {
			return ip.String()
		}
	}
	return host
}
