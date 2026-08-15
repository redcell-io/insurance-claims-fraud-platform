// Package correlation generates request correlation IDs.
//
// A dependency-free stand-in for a UUID library, adequate for the walking
// skeleton. Revisit if/when OpenTelemetry wiring lands (see DESIGN.md §11)
// — trace/span IDs may supersede this.
package correlation

import (
	"crypto/rand"
	"encoding/hex"
)

// New returns a random 16-byte correlation ID, hex-encoded.
func New() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failing is effectively unrecoverable on any real
		// platform; fall back to a fixed marker rather than panicking the
		// request path.
		return "correlation-id-unavailable"
	}
	return hex.EncodeToString(b)
}
