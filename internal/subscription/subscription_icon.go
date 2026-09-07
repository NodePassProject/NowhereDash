package subscription

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"errors"
	"image/png"
	"strings"
)

const (
	subscriptionIconSize    = 96
	maxSubscriptionIconSize = 32 * 1024
)

var errInvalidSubscriptionIcon = errors.New("icon must be a square 96x96 PNG no larger than 32 KiB")

// The 96x96 defaults are derived from web/public/logo*.png so public
// subscription responses work from the packaged binary without frontend access.
//
//go:embed logo.png
var defaultSubscriptionIconLightPNG []byte

//go:embed logo-dark.png
var defaultSubscriptionIconDarkPNG []byte

var (
	defaultSubscriptionIconLightBase64 = base64.StdEncoding.EncodeToString(defaultSubscriptionIconLightPNG)
	defaultSubscriptionIconDarkBase64  = base64.StdEncoding.EncodeToString(defaultSubscriptionIconDarkPNG)
)

func decodeSubscriptionIcon(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}

	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(strings.ToLower(value), prefix) {
		return nil, errInvalidSubscriptionIcon
	}
	encoded := value[len(prefix):]
	if len(encoded) > base64.StdEncoding.EncodedLen(maxSubscriptionIconSize) {
		return nil, errInvalidSubscriptionIcon
	}

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(decoded) == 0 || len(decoded) > maxSubscriptionIconSize {
		return nil, errInvalidSubscriptionIcon
	}
	configuration, err := png.DecodeConfig(bytes.NewReader(decoded))
	if err != nil || configuration.Width != subscriptionIconSize || configuration.Height != subscriptionIconSize {
		return nil, errInvalidSubscriptionIcon
	}

	return decoded, nil
}

func subscriptionIconDataURL(icon []byte) string {
	if len(icon) == 0 {
		return "/logo.png"
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(icon)
}

func subscriptionIconBase64Pair(icon []byte) (light string, dark string) {
	if len(icon) == 0 {
		return defaultSubscriptionIconLightBase64, defaultSubscriptionIconDarkBase64
	}
	encoded := base64.StdEncoding.EncodeToString(icon)
	return encoded, encoded
}
