package nowhere

import (
	"fmt"
	"strings"
)

// VersionAtLeast reports whether actual is greater than or equal to required.
// Version values are internal capability inputs and are not exposed to API clients.
func VersionAtLeast(actual, required string) bool {
	actualParts := strings.Split(strings.TrimPrefix(actual, "v"), ".")
	requiredParts := strings.Split(strings.TrimPrefix(required, "v"), ".")

	for len(actualParts) < len(requiredParts) {
		actualParts = append(actualParts, "0")
	}
	for len(requiredParts) < len(actualParts) {
		requiredParts = append(requiredParts, "0")
	}

	for index := range actualParts {
		var actualNumber, requiredNumber int
		_, _ = fmt.Sscanf(actualParts[index], "%d", &actualNumber)
		_, _ = fmt.Sscanf(requiredParts[index], "%d", &requiredNumber)
		if actualNumber != requiredNumber {
			return actualNumber > requiredNumber
		}
	}

	return true
}
