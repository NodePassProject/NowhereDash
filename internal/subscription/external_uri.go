package subscription

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"github.com/google/uuid"
)

const (
	maxExternalNodeCount = 500
	maxExternalURILength = 8192
)

var supportedExternalSchemes = map[string]struct{}{
	"nowhere": {}, "vless": {}, "hysteria2": {}, "hy2": {},
	"trojan": {}, "anytls": {}, "ss": {}, "socks5": {},
	"socks": {}, "sudoku": {},
}

type externalNodeValue struct {
	Scheme string
	URI    string
}

func validateExternalURIs(values []string) ([]externalNodeValue, error) {
	if len(values) > maxExternalNodeCount {
		return nil, fmt.Errorf("externalUris must contain at most %d entries", maxExternalNodeCount)
	}
	nodes := make([]externalNodeValue, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		raw := strings.TrimSpace(value)
		if raw == "" {
			return nil, fmt.Errorf("externalUris[%d] must not be empty", index)
		}
		if len([]byte(raw)) > maxExternalURILength {
			return nil, fmt.Errorf("externalUris[%d] exceeds %d bytes", index, maxExternalURILength)
		}
		if strings.ContainsAny(raw, "\r\n") || strings.IndexFunc(raw, unicode.IsControl) >= 0 {
			return nil, fmt.Errorf("externalUris[%d] contains control characters", index)
		}
		if _, duplicate := seen[raw]; duplicate {
			return nil, fmt.Errorf("externalUris[%d] duplicates an earlier URI", index)
		}
		scheme, err := validateExternalURI(raw)
		if err != nil {
			return nil, fmt.Errorf("externalUris[%d]: %w", index, err)
		}
		seen[raw] = struct{}{}
		nodes = append(nodes, externalNodeValue{Scheme: scheme, URI: raw})
	}
	return nodes, nil
}

func validateExternalURI(raw string) (string, error) {
	separator := strings.Index(raw, "://")
	if separator <= 0 {
		return "", errors.New("URI must contain a supported scheme")
	}
	scheme := raw[:separator]
	if scheme != strings.ToLower(scheme) {
		return "", errors.New("URI scheme must be lowercase")
	}
	if _, supported := supportedExternalSchemes[scheme]; !supported {
		return "", fmt.Errorf("unsupported URI scheme %q", scheme)
	}

	var err error
	switch scheme {
	case "ss":
		err = validateShadowsocksURI(raw)
	case "socks", "socks5":
		err = validateSOCKSURI(raw, scheme)
	case "sudoku":
		err = validateSudokuURI(raw)
	default:
		userinfo, _, _, splitErr := splitExternalURI(raw, scheme, scheme == "vless")
		if splitErr != nil {
			err = splitErr
			break
		}
		decodedUser, decodeErr := url.PathUnescape(userinfo)
		if decodeErr != nil {
			err = errors.New("invalid user information encoding")
			break
		}
		if decodedUser == "" {
			err = errors.New("missing credential")
			break
		}
		if scheme == "vless" {
			if _, parseErr := uuid.Parse(decodedUser); parseErr != nil {
				err = errors.New("invalid VLESS UUID")
			}
		}
		if scheme == "nowhere" && strings.Contains(decodedUser, ":") {
			err = errors.New("invalid Nowhere key encoding")
		}
	}
	if err != nil {
		return "", err
	}
	return scheme, nil
}

func splitExternalURI(raw, scheme string, allowBase64Body bool) (string, string, uint16, error) {
	body := strings.TrimPrefix(raw, scheme+"://")
	if hash := strings.LastIndex(body, "#"); hash >= 0 {
		body = body[:hash]
	}
	if allowBase64Body && !strings.Contains(body, "@") {
		if decoded, ok := decodeBase64String(body); ok && strings.Contains(decoded, "@") {
			body = decoded
			if hash := strings.LastIndex(body, "#"); hash >= 0 {
				body = body[:hash]
			}
		}
	}
	if question := strings.Index(body, "?"); question >= 0 {
		body = body[:question]
	}
	at := strings.LastIndex(body, "@")
	if at < 0 {
		return "", "", 0, errors.New("missing @ separator")
	}
	userinfo := body[:at]
	server := strings.TrimSuffix(body[at+1:], "/")
	if slash := strings.Index(server, "/"); slash >= 0 {
		server = server[:slash]
	}
	host, port, err := parseExternalHostPort(server)
	return userinfo, host, port, err
}

func parseExternalHostPort(value string) (string, uint16, error) {
	host, rawPort, err := net.SplitHostPort(value)
	if err != nil {
		return "", 0, errors.New("invalid or missing host and port")
	}
	if strings.TrimSpace(host) == "" || strings.IndexFunc(host, unicode.IsSpace) >= 0 ||
		strings.ContainsAny(host, "/\\@?#") {
		return "", 0, errors.New("missing host")
	}
	port, err := strconv.ParseUint(rawPort, 10, 16)
	if err != nil || port == 0 {
		return "", 0, errors.New("invalid port")
	}
	return host, uint16(port), nil
}

func validateShadowsocksURI(raw string) error {
	body := strings.TrimPrefix(raw, "ss://")
	if hash := strings.LastIndex(body, "#"); hash >= 0 {
		body = body[:hash]
	}
	if question := strings.Index(body, "?"); question >= 0 {
		query, err := url.ParseQuery(body[question+1:])
		if err != nil {
			return errors.New("invalid Shadowsocks query")
		}
		if query.Get("plugin") != "" {
			return errors.New("Shadowsocks plugins are not supported by Anywhere")
		}
		body = body[:question]
	}

	var userinfo, server string
	if at := strings.LastIndex(body, "@"); at >= 0 {
		userinfo, server = body[:at], body[at+1:]
	} else {
		decoded, ok := decodeBase64String(body)
		if !ok {
			return errors.New("invalid Shadowsocks Base64 body")
		}
		at := strings.LastIndex(decoded, "@")
		if at < 0 {
			return errors.New("missing Shadowsocks server")
		}
		userinfo, server = decoded[:at], decoded[at+1:]
	}
	if slash := strings.Index(server, "/"); slash >= 0 {
		server = server[:slash]
	}
	if _, _, err := parseExternalHostPort(server); err != nil {
		return err
	}
	methodAndPassword := userinfo
	if !strings.Contains(methodAndPassword, ":") {
		decoded, ok := decodeBase64String(methodAndPassword)
		if !ok {
			return errors.New("invalid Shadowsocks user information")
		}
		methodAndPassword = decoded
	}
	separator := strings.Index(methodAndPassword, ":")
	if separator <= 0 || separator == len(methodAndPassword)-1 {
		return errors.New("Shadowsocks URI must contain method and password")
	}
	method := strings.ToLower(methodAndPassword[:separator])
	supportedMethods := map[string]struct{}{
		"aes-128-gcm": {}, "aes-256-gcm": {}, "chacha20-ietf-poly1305": {},
		"chacha20-poly1305": {}, "none": {}, "plain": {},
		"2022-blake3-aes-128-gcm": {}, "2022-blake3-aes-256-gcm": {},
		"2022-blake3-chacha20-poly1305": {},
	}
	if _, ok := supportedMethods[method]; !ok {
		return fmt.Errorf("unsupported Shadowsocks method %q", method)
	}
	return nil
}

func validateSOCKSURI(raw, scheme string) error {
	body := strings.TrimPrefix(raw, scheme+"://")
	if hash := strings.LastIndex(body, "#"); hash >= 0 {
		body = body[:hash]
	}
	if at := strings.LastIndex(body, "@"); at >= 0 {
		body = body[at+1:]
	}
	if slash := strings.Index(body, "/"); slash >= 0 {
		body = body[:slash]
	}
	_, _, err := parseExternalHostPort(body)
	return err
}

func validateSudokuURI(raw string) error {
	body := strings.TrimPrefix(raw, "sudoku://")
	decoded, ok := decodeBase64Bytes(body)
	if !ok {
		return errors.New("invalid Sudoku Base64 body")
	}
	var payload struct {
		Host string      `json:"h"`
		Port json.Number `json:"p"`
		Key  string      `json:"k"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(decoded)))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return errors.New("invalid Sudoku JSON body")
	}
	if strings.TrimSpace(payload.Host) == "" || payload.Key == "" {
		return errors.New("Sudoku URI is missing host or key")
	}
	port, err := strconv.ParseUint(string(payload.Port), 10, 16)
	if err != nil || port == 0 {
		return errors.New("invalid Sudoku port")
	}
	return nil
}

func decodeBase64String(value string) (string, bool) {
	decoded, ok := decodeBase64Bytes(value)
	if !ok {
		return "", false
	}
	return string(decoded), true
}

func decodeBase64Bytes(value string) ([]byte, bool) {
	value = strings.TrimSpace(value)
	encodings := []*base64.Encoding{
		base64.RawURLEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.StdEncoding,
	}
	for _, encoding := range encodings {
		if decoded, err := encoding.DecodeString(value); err == nil {
			return decoded, true
		}
	}
	return nil, false
}
