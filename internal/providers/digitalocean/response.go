package digitalocean

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

const maxResponseBytes = 8 << 20

// Decode exactly one bounded value. A partial/oversized response is an unknown
// outcome for mutations, never permission to send the same creation again.
func decodeProviderResponse(body io.Reader, out any) error {
	raw, err := io.ReadAll(io.LimitReader(body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("%w: response read", ErrProviderRequest)
	}
	if len(raw) > maxResponseBytes {
		return fmt.Errorf("%w: response too large", ErrProviderRequest)
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, out) != nil {
		return fmt.Errorf("%w: decode", ErrProviderRequest)
	}
	return nil
}
