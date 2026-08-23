package endpoint

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"NowhereDash/internal/nowhere"
)

const maxRegistrationTokens = 1024

var (
	ErrInvalidRegistrationToken  = errors.New("invalid or expired registration token")
	ErrRegistrationTokenBusy     = errors.New("registration token is already in use")
	ErrTooManyRegistrationTokens = errors.New("too many active registration tokens")
)

// RegistrationClaims are bound to a short-lived installer token.
type RegistrationClaims struct {
	Name      string
	ExpiresAt time.Time
}

type registrationTokenEntry struct {
	claims RegistrationClaims
	inUse  bool
}

// RegistrationTokenStore keeps only token hashes in memory. Tokens become
// invalid when NowhereDash restarts and are consumed after one successful use.
type RegistrationTokenStore struct {
	mu     sync.Mutex
	ttl    time.Duration
	now    func() time.Time
	tokens map[[sha256.Size]byte]*registrationTokenEntry
}

func NewRegistrationTokenStore(ttl time.Duration) *RegistrationTokenStore {
	return &RegistrationTokenStore{
		ttl:    ttl,
		now:    time.Now,
		tokens: make(map[[sha256.Size]byte]*registrationTokenEntry),
	}
}

func validateRegistrationName(name string) error {
	if name == "" {
		return errors.New("endpoint name is required")
	}
	if utf8.RuneCountInString(name) > 50 {
		return errors.New("endpoint name must not exceed 50 characters")
	}
	if strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return errors.New("endpoint name contains control characters")
	}
	return nil
}

func (s *RegistrationTokenStore) purgeExpiredLocked(now time.Time) {
	for digest, entry := range s.tokens {
		if !now.Before(entry.claims.ExpiresAt) {
			delete(s.tokens, digest)
		}
	}
}

// Issue creates a cryptographically random, URL-safe token bound to a name.
func (s *RegistrationTokenStore) Issue(name string) (string, RegistrationClaims, error) {
	name = strings.TrimSpace(name)
	if err := validateRegistrationName(name); err != nil {
		return "", RegistrationClaims{}, err
	}
	if s.ttl <= 0 {
		return "", RegistrationClaims{}, errors.New("registration token TTL must be positive")
	}

	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeExpiredLocked(now)
	if len(s.tokens) >= maxRegistrationTokens {
		return "", RegistrationClaims{}, ErrTooManyRegistrationTokens
	}

	for {
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return "", RegistrationClaims{}, fmt.Errorf("generate registration token: %w", err)
		}
		token := base64.RawURLEncoding.EncodeToString(raw)
		digest := sha256.Sum256([]byte(token))
		if _, exists := s.tokens[digest]; exists {
			continue
		}

		claims := RegistrationClaims{Name: name, ExpiresAt: now.Add(s.ttl)}
		s.tokens[digest] = &registrationTokenEntry{claims: claims}
		return token, claims, nil
	}
}

// Use reserves a token while fn runs. A successful callback consumes it;
// failures release it so a corrected installer callback can retry.
func (s *RegistrationTokenStore) Use(token string, fn func(RegistrationClaims) error) error {
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(token))
	if err != nil || len(decoded) != 32 {
		return ErrInvalidRegistrationToken
	}
	digest := sha256.Sum256([]byte(strings.TrimSpace(token)))

	now := s.now()
	s.mu.Lock()
	s.purgeExpiredLocked(now)
	entry, exists := s.tokens[digest]
	if !exists {
		s.mu.Unlock()
		return ErrInvalidRegistrationToken
	}
	if entry.inUse {
		s.mu.Unlock()
		return ErrRegistrationTokenBusy
	}
	entry.inUse = true
	claims := entry.claims
	s.mu.Unlock()

	callbackErr := fn(claims)

	s.mu.Lock()
	defer s.mu.Unlock()
	current, stillExists := s.tokens[digest]
	if !stillExists || current != entry {
		return callbackErr
	}
	if callbackErr == nil {
		delete(s.tokens, digest)
	} else if !s.now().Before(entry.claims.ExpiresAt) {
		delete(s.tokens, digest)
	} else {
		entry.inUse = false
	}
	return callbackErr
}

// InstallerRegistrationRequest is submitted by install.sh after OpenCtrl starts.
type InstallerRegistrationRequest struct {
	Token    string `json:"token"`
	APIURL   string `json:"apiUrl"`
	APIKey   string `json:"apiKey"`
	Hostname string `json:"hostname,omitempty"`
}

// BuildInstallerEndpointRequest validates installer-controlled data and splits
// the complete OpenCtrl API URL into the endpoint base URL and API path.
func BuildInstallerEndpointRequest(req InstallerRegistrationRequest) (CreateEndpointRequest, error) {
	apiURL := strings.TrimSpace(req.APIURL)
	parsed, err := url.Parse(apiURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return CreateEndpointRequest{}, errors.New("invalid OpenCtrl API URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return CreateEndpointRequest{}, errors.New("OpenCtrl API URL must use HTTP or HTTPS")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" {
		return CreateEndpointRequest{}, errors.New("OpenCtrl API URL contains unsupported components")
	}
	if parsed.Hostname() == "" || strings.ContainsAny(parsed.Host, "\r\n\t ") {
		return CreateEndpointRequest{}, errors.New("invalid OpenCtrl API URL host")
	}
	if parsed.RawPath != "" || strings.Contains(parsed.Path, "//") {
		return CreateEndpointRequest{}, errors.New("invalid OpenCtrl API path")
	}

	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey == "" {
		return CreateEndpointRequest{}, errors.New("OpenCtrl API key is required")
	}
	if len(apiKey) > 200 || strings.ContainsAny(apiKey, "\r\n") {
		return CreateEndpointRequest{}, errors.New("invalid OpenCtrl API key")
	}

	hostname := strings.TrimSpace(req.Hostname)
	if hostname != "" {
		trimmedHost := strings.TrimSuffix(strings.TrimPrefix(hostname, "["), "]")
		if trimmedHost == "" || len(hostname) > 255 || strings.ContainsAny(hostname, "/?#@\r\n\t ") {
			return CreateEndpointRequest{}, errors.New("invalid endpoint hostname")
		}
	}

	baseURL := (&url.URL{Scheme: parsed.Scheme, Host: parsed.Host}).String()
	return CreateEndpointRequest{
		URL:      baseURL,
		APIPath:  nowhere.NormalizeAPIPath(parsed.Path),
		APIKey:   apiKey,
		Hostname: hostname,
	}, nil
}
