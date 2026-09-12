package nowhere

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

func decodeQueryComponent(value string) (string, error) {
	decoded, err := url.PathUnescape(value)
	if err != nil || !utf8.ValidString(decoded) {
		return "", fmt.Errorf("invalid query encoding")
	}
	return decoded, nil
}

// Native queries preserve '+' and parse nested authority delimiters before
// decoding credentials. Form query encoders cannot represent those semantics.
func portalQuery(parsed *url.URL) (url.Values, error) {
	values := url.Values{}
	read := func(keys []string) error {
		for _, pair := range strings.Split(parsed.RawQuery, "&") {
			rawKey, rawValue, _ := strings.Cut(pair, "=")
			key, err := decodeQueryComponent(rawKey)
			if err != nil || !slices.Contains(keys, key) || values.Has(key) {
				continue
			}
			value, err := decodeQueryComponent(rawValue)
			if err != nil {
				return err
			}
			if (key == "next" || key == "socks") && value != "none" {
				value = rawValue
			}
			values.Set(key, value)
		}
		return nil
	}
	if err := read([]string{"tls", "crt", "key", "morph", "rate", "etar", "dial", "socks", "next", "log"}); err != nil {
		return values, err
	}
	if next := values.Get("next"); next != "" && next != "none" {
		if err := read([]string{"up", "down", "mux", "sni", "pin"}); err != nil {
			return values, err
		}
	}
	return values, nil
}

func encodeNativeQuery(values url.Values) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		value := values.Get(key)
		if key != "next" && key != "socks" {
			value = strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
		}
		pairs = append(pairs, url.QueryEscape(key)+"="+value)
	}
	return strings.Join(pairs, "&")
}

func parseSocksEndpoint(value string, allowEmptyHost bool) (string, error) {
	if strings.ContainsAny(value, "&?# \t\r\n") {
		return "", fmt.Errorf("reserved SOCKS characters must be percent-encoded")
	}
	endpoint := value
	if rawCredentials, rawEndpoint, authenticated := strings.Cut(value, "@"); authenticated {
		if strings.Contains(rawEndpoint, "@") {
			return "", fmt.Errorf("socks requires USERNAME:PASSWORD@HOST:PORT")
		}
		username, password, ok := strings.Cut(rawCredentials, ":")
		if !ok {
			return "", fmt.Errorf("socks requires USERNAME:PASSWORD@HOST:PORT")
		}
		for _, credential := range []string{username, password} {
			decoded, err := decodeQueryComponent(credential)
			if err != nil || len(decoded) < 1 || len(decoded) > 255 || strings.ContainsAny(credential, ":/?#[]@!$&'()*+,;= \t\r\n") {
				return "", fmt.Errorf("SOCKS credentials require 1 to 255 bytes and percent-encoded reserved characters")
			}
		}
		endpoint = rawEndpoint
	}
	decoded, err := decodeQueryComponent(endpoint)
	if err != nil {
		return "", err
	}
	host, port, err := net.SplitHostPort(decoded)
	if err != nil || (!allowEmptyHost && host == "") || strings.ContainsAny(host, "/@?#%[]<>\\ \t\r\n") {
		return "", fmt.Errorf("socks requires a valid HOST:PORT")
	}
	if strings.Contains(host, ":") {
		ip, err := netip.ParseAddr(host)
		if err != nil || !ip.Is6() || ip.Zone() != "" {
			return "", fmt.Errorf("invalid SOCKS IPv6 address")
		}
	}
	number, err := strconv.ParseUint(port, 10, 16)
	if err != nil || number == 0 {
		return "", fmt.Errorf("SOCKS port must be between 1 and 65535")
	}
	return net.JoinHostPort(host, strconv.FormatUint(number, 10)), nil
}

func validateSNI(value string) error {
	if value == "" || value == "none" {
		return nil
	}
	_, ipErr := netip.ParseAddr(value)
	if len(value) > 253 || ipErr == nil || strings.ContainsAny(value, ":[]") || strings.IndexFunc(value, func(r rune) bool { return r > 127 }) >= 0 {
		return fmt.Errorf("sni must be an ASCII DNS name")
	}
	return nil
}
