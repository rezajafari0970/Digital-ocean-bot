package clientops

import (
	"encoding/json"
	"errors"
	"strings"
)

var ErrCanaryGuard = errors.New("client mutation canary guard rejected")

const CanaryEmailPrefix = "clientops-canary-"

func ValidateCanary(job Job) error {
	if job.State != StatePending || job.Attempts != 0 {
		return ErrCanaryGuard
	}
	if job.Kind != KindCreate && job.Kind != KindDelete {
		return ErrCanaryGuard
	}
	if !strings.HasPrefix(job.IdempotencyKey, "guarded-canary-") {
		return ErrCanaryGuard
	}
	if job.Kind == KindCreate {
		client, err := payloadClient(job.Payload)
		if err != nil {
			return ErrCanaryGuard
		}
		if client.ID != job.ClientID || !strings.HasPrefix(client.Email, CanaryEmailPrefix) {
			return ErrCanaryGuard
		}
	}
	return nil
}

func CanaryPayload(clientID string) json.RawMessage {
	if len(clientID) < 8 {
		return nil
	}
	payload, _ := json.Marshal(map[string]any{
		"Client": map[string]any{
			"id": clientID, "email": CanaryEmailPrefix + clientID[:8],
			"enable": true, "totalGB": 0, "expiryTime": 0,
			"limitIp": 0, "flow": "xtls-rprx-vision",
		},
	})
	return payload
}
