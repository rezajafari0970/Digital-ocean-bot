package sanaei

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/inventory"
)

var ErrInventoryRejected = errors.New("panel inventory rejected")

type inventoryEnvelope struct {
	Success bool              `json:"success"`
	Obj     []json.RawMessage `json:"obj"`
}

type inboundSummary struct {
	ID int64 `json:"id"`

	Remark   string `json:"remark"`
	Protocol string `json:"protocol"`

	Port   int    `json:"port"`
	Listen string `json:"listen"`

	Enable bool `json:"enable"`

	Up    int64 `json:"up"`
	Down  int64 `json:"down"`
	Total int64 `json:"total"`

	Settings json.RawMessage `json:"settings"`

	StreamSettings json.RawMessage `json:"streamSettings"`

	ClientStats []json.RawMessage `json:"clientStats"`
}

func ReadInventory(
	ctx context.Context,
	exec SessionExecutor,
	panelID string,
) (inventory.Snapshot, error) {

	if exec == nil ||
		panelID == "" {

		return inventory.Snapshot{},
			ErrSessionRequest
	}

	resp, err := exec.Do(
		ctx,
		SessionRequest{
			Method: http.MethodGet,
			Path:   "panel/api/inbounds/list/slim",
		},
	)

	if err != nil {
		return inventory.Snapshot{}, err
	}

	if resp.StatusCode == http.StatusNotFound ||
		resp.StatusCode == http.StatusMethodNotAllowed {

		resp, err = exec.Do(
			ctx,
			SessionRequest{
				Method: http.MethodGet,
				Path:   "panel/api/inbounds/list",
			},
		)

		if err != nil {
			return inventory.Snapshot{}, err
		}
	}

	if resp.StatusCode < 200 ||
		resp.StatusCode >= 300 {

		return inventory.Snapshot{},
			ErrSessionRequest
	}

	var envelope inventoryEnvelope

	if err = json.Unmarshal(
		resp.Body,
		&envelope,
	); err != nil {

		return inventory.Snapshot{}, err
	}

	if !envelope.Success {
		return inventory.Snapshot{},
			ErrInventoryRejected
	}

	now := time.Now().UTC()

	snapshot := inventory.Snapshot{
		PanelID:    panelID,
		ObservedAt: now,

		Records: make(
			[]inventory.InboundRecord,
			0,
			len(envelope.Obj),
		),
	}

	for _, raw := range envelope.Obj {
		var inbound inboundSummary

		if err = json.Unmarshal(
			raw,
			&inbound,
		); err != nil {

			return inventory.Snapshot{}, err
		}

		hash := sha256.Sum256(
			raw,
		)

		transport, security :=
			streamSummary(
				inbound.StreamSettings,
			)

		snapshot.Records = append(
			snapshot.Records,

			inventory.InboundRecord{
				PanelID: panelID,

				RemoteID: inbound.ID,

				Remark: inbound.Remark,

				Protocol: inbound.Protocol,

				Port: inbound.Port,

				Listen: inbound.Listen,

				Enabled: inbound.Enable,

				Transport: transport,

				Security: security,

				ClientCount: len(
					inbound.ClientStats,
				),

				Upload: inbound.Up,

				Download: inbound.Down,

				Total: inbound.Total,

				RawHash: hex.EncodeToString(
					hash[:],
				),

				ObservedAt: now,
			},
		)
	}

	return snapshot, nil
}

func streamSummary(
	raw json.RawMessage,
) (string, string) {

	if len(raw) == 0 ||
		string(raw) == "null" {

		return "", ""
	}

	var payload []byte

	// 3x-ui commonly serializes streamSettings
	// as a JSON string.
	var encoded string

	if err := json.Unmarshal(
		raw,
		&encoded,
	); err == nil {

		payload = []byte(
			encoded,
		)

	} else {

		// Be tolerant of versions returning
		// streamSettings as an object.
		payload = raw
	}

	var summary struct {
		Network  string `json:"network"`
		Security string `json:"security"`
	}

	if err := json.Unmarshal(
		payload,
		&summary,
	); err != nil {

		return "", ""
	}

	return summary.Network,
		summary.Security
}
