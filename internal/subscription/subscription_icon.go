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

// defaultSubscriptionIconPNG is embedded so public subscription responses work
// from the packaged server binary without frontend filesystem access.
//
//go:embed nowhere-icon.png
var defaultSubscriptionIconPNG []byte

var defaultSubscriptionIconBase64 = base64.StdEncoding.EncodeToString(defaultSubscriptionIconPNG)

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
		return "/nowhere-icon.png"
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(icon)
}

func subscriptionIconBase64(icon []byte) string {
	if len(icon) == 0 {
		return defaultSubscriptionIconBase64
	}
	return base64.StdEncoding.EncodeToString(icon)
}
