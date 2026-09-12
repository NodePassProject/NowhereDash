package nowhere

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"NowhereDash/internal/models"
)

// ServiceEndpoint is the shared Portal, Vector and native next-hop endpoint.
type ServiceEndpoint struct {
	Host      string
	TCPPort   string
	UDPPort   string
	TCPFamily string
	UDPFamily string
}

func endpointFamily(family string) string {
	if family == "" {
		return "any"
	}
	return family
}

func (endpoint ServiceEndpoint) Validate(allowWildcard bool) error {
	if endpoint.Host == "" || (!allowWildcard && endpoint.Host == "*") || strings.ContainsAny(endpoint.Host, "/@?#%[]<>\\") || strings.IndexFunc(endpoint.Host, func(r rune) bool { return r <= 32 || r == 127 }) >= 0 {
		return fmt.Errorf("endpoint requires a valid host; wildcard is only allowed for Portal listeners")
	}
	ip, ipErr := netip.ParseAddr(endpoint.Host)
	if strings.Contains(endpoint.Host, ":") && (ipErr != nil || ip.Zone() != "") {
		return fmt.Errorf("invalid endpoint IP address")
	}
	if endpoint.TCPPort == "" && endpoint.UDPPort == "" {
		return fmt.Errorf("at least one TCP or UDP listener is required")
	}
	for _, carrier := range []struct{ name, port, family string }{
		{"tcp", endpoint.TCPPort, endpoint.TCPFamily}, {"udp", endpoint.UDPPort, endpoint.UDPFamily},
	} {
		if carrier.port == "" {
			continue
		}
		port, err := strconv.ParseUint(carrier.port, 10, 16)
		if err != nil || port == 0 || strings.IndexFunc(carrier.port, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return fmt.Errorf("%s port must be between 1 and 65535", carrier.name)
		}
		family := endpointFamily(carrier.family)
		if family != "any" && family != "4" && family != "6" {
			return fmt.Errorf("%s address family must be any, 4, or 6", carrier.name)
		}
		if ipErr == nil && ((family == "4" && !ip.Is4()) || (family == "6" && !ip.Is6())) {
			return fmt.Errorf("%s address family conflicts with the endpoint IP", carrier.name)
		}
	}
	return nil
}

func (endpoint ServiceEndpoint) Network() string {
	if endpoint.TCPPort == "" {
		return "udp"
	}
	if endpoint.UDPPort == "" {
		return "tcp"
	}
	return "mix"
}

func (endpoint ServiceEndpoint) DefaultCarrier() string {
	if endpoint.TCPPort != "" {
		return "tcp"
	}
	return "udp"
}

func (endpoint ServiceEndpoint) Canonical() string {
	if ip, err := netip.ParseAddr(endpoint.Host); err == nil {
		endpoint.Host = ip.String()
	}
	if endpoint.TCPPort != "" && endpoint.TCPPort == endpoint.UDPPort &&
		endpointFamily(endpoint.TCPFamily) == "any" && endpointFamily(endpoint.UDPFamily) == "any" {
		return net.JoinHostPort(endpoint.Host, endpoint.TCPPort)
	}
	host := endpoint.Host
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	for _, carrier := range []struct{ name, port, family string }{
		{"tcp", endpoint.TCPPort, endpoint.TCPFamily}, {"udp", endpoint.UDPPort, endpoint.UDPFamily},
	} {
		if carrier.port != "" {
			family := endpointFamily(carrier.family)
			if family == "any" {
				family = ""
			}
			host += "/" + carrier.name + family + ":" + carrier.port
		}
	}
	return host
}

func parseServiceEndpoint(parsed *url.URL, allowWildcard bool) (ServiceEndpoint, error) {
	endpoint := ServiceEndpoint{Host: parsed.Hostname(), TCPFamily: "any", UDPFamily: "any"}
	if strings.Contains(endpoint.Host, ":") && !strings.HasPrefix(parsed.Host, "[") {
		return endpoint, fmt.Errorf("IPv6 endpoint addresses must be enclosed in brackets")
	}
	if parsed.Path == "" {
		endpoint.TCPPort, endpoint.UDPPort = parsed.Port(), parsed.Port()
		if endpoint.Host == "" && allowWildcard {
			endpoint.Host = "*"
		}
	} else {
		if parsed.Port() != "" {
			return endpoint, fmt.Errorf("authority port and carrier path cannot be combined")
		}
		for _, segment := range strings.Split(strings.TrimPrefix(parsed.EscapedPath(), "/"), "/") {
			carrier, port, ok := strings.Cut(segment, ":")
			if !ok || port == "" {
				return endpoint, fmt.Errorf("each endpoint path segment must be CARRIER:PORT")
			}
			var portSlot, familySlot *string
			switch carrier {
			case "tcp", "tcp4", "tcp6":
				portSlot, familySlot = &endpoint.TCPPort, &endpoint.TCPFamily
			case "udp", "udp4", "udp6":
				portSlot, familySlot = &endpoint.UDPPort, &endpoint.UDPFamily
			default:
				return endpoint, fmt.Errorf("unknown endpoint carrier")
			}
			if *portSlot != "" {
				return endpoint, fmt.Errorf("each carrier may only appear once")
			}
			*portSlot = port
			if len(carrier) == 4 {
				*familySlot = carrier[3:]
			}
		}
	}
	if err := endpoint.Validate(allowWildcard); err != nil {
		return endpoint, err
	}
	for _, port := range []*string{&endpoint.TCPPort, &endpoint.UDPPort} {
		if *port != "" {
			number, _ := strconv.Atoi(*port)
			*port = strconv.Itoa(number)
		}
	}
	return endpoint, nil
}

// EndpointFromTunnel retains the carrier selection of older stored entities.
// URL parsing itself follows v2: the old net query has no effect.
func EndpointFromTunnel(tunnel models.Tunnel) ServiceEndpoint {
	endpoint := ServiceEndpoint{
		Host: normalizeHost(tunnel.ListenHost), TCPPort: valueOr(tunnel.TCPPort, ""), UDPPort: valueOr(tunnel.UDPPort, ""),
		TCPFamily: endpointFamily(tunnel.TCPFamily), UDPFamily: endpointFamily(tunnel.UDPFamily),
	}
	if endpoint.Host == "" {
		endpoint.Host = "*"
	}
	if tunnel.TCPPort == nil && tunnel.UDPPort == nil {
		network := valueOr(tunnel.Network, "mix")
		if network != "udp" {
			endpoint.TCPPort = tunnel.ListenPort
		}
		if network != "tcp" {
			endpoint.UDPPort = tunnel.ListenPort
		}
	}
	return endpoint
}

func applyEndpoint(tunnel *models.Tunnel, endpoint ServiceEndpoint) {
	tunnel.ListenHost = endpoint.Host
	tunnel.TCPPort, tunnel.UDPPort = &endpoint.TCPPort, &endpoint.UDPPort
	tunnel.TCPFamily, tunnel.UDPFamily = endpoint.TCPFamily, endpoint.UDPFamily
	tunnel.ListenPort = endpoint.TCPPort
	if tunnel.ListenPort == "" {
		tunnel.ListenPort = endpoint.UDPPort
	}
	network := endpoint.Network()
	tunnel.Network = &network
}

func parseNextEndpoint(value string) (ServiceEndpoint, error) {
	if strings.Count(value, "@") != 1 || strings.ContainsAny(value, "?#& \t\r\n") {
		return ServiceEndpoint{}, fmt.Errorf("next must contain one encoded shared key and endpoint, without a query or fragment")
	}
	parsed, err := url.Parse("vector://" + value)
	if err != nil || parsed.User == nil || parsed.User.Username() == "" || len([]byte(parsed.User.Username())) > 255 {
		return ServiceEndpoint{}, fmt.Errorf("invalid next endpoint")
	}
	if _, password := parsed.User.Password(); password {
		return ServiceEndpoint{}, fmt.Errorf("next does not accept password userinfo")
	}
	return parseServiceEndpoint(parsed, false)
}

func nextDefaultCarrier(next *string) string {
	if parsed, err := url.Parse("vector://" + withoutAuthorityCredential(valueOr(next, "none"))); err == nil {
		if endpoint, err := parseServiceEndpoint(parsed, false); err == nil {
			return endpoint.DefaultCarrier()
		}
	}
	return "tcp"
}

// ParsePortalURL is used for user input; runtime parsing may contain redactions.
func ParsePortalURL(raw string) (*models.Tunnel, error) {
	parsed, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(parsed.Scheme, "portal") || parsed.User == nil || strings.Contains(raw, "#") {
		return nil, fmt.Errorf("a portal URL with a shared key and no fragment is required")
	}
	if _, password := parsed.User.Password(); password {
		return nil, fmt.Errorf("Portal URLs do not accept password userinfo")
	}
	if _, err := parseServiceEndpoint(parsed, true); err != nil {
		return nil, err
	}
	query, err := portalQuery(parsed)
	if err != nil {
		return nil, fmt.Errorf("invalid Portal query encoding")
	}
	for _, key := range []string{"tls", "crt", "key", "morph", "rate", "etar", "dial", "socks", "next", "log"} {
		if query.Has(key) && query.Get(key) == "" {
			return nil, fmt.Errorf("%s cannot be empty", key)
		}
	}
	for _, key := range []string{"rate", "etar"} {
		if query.Has(key) && int64Ptr(query.Get(key)) == nil {
			return nil, fmt.Errorf("%s must be a nonnegative integer", key)
		}
	}
	if next := query.Get("next"); next != "" && next != "none" {
		for _, key := range []string{"up", "down", "mux"} {
			if query.Has(key) && query.Get(key) == "" {
				return nil, fmt.Errorf("%s cannot be empty when next is enabled", key)
			}
		}
	}
	tunnel := ParseTunnelURL(raw)
	return tunnel, ValidatePortalTunnel(*tunnel)
}
