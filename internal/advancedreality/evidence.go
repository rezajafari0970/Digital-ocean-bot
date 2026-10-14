package advancedreality

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"time"
)

var ErrEvidence = errors.New("invalid_connectivity_evidence")

// Evidence is a bounded report from an operator, not an authenticated device
// attestation. Hashes bind one exact test context; they are never credentials.
// No result from this assessor grants a deployment or mutation permission.
type Evidence struct {
	Schema           int                `json:"schema"`
	StartedAt        time.Time          `json:"started_at"`
	FinishedAt       time.Time          `json:"finished_at"`
	Scope            string             `json:"scope"`
	ClientVersion    string             `json:"client_version"`
	ClientCoreSHA256 string             `json:"client_core_sha256"`
	ServerCoreSHA256 string             `json:"server_core_sha256"`
	ConfigSHA256     string             `json:"config_sha256"`
	TargetSHA256     string             `json:"target_sha256"`
	NetworkASN       uint32             `json:"network_asn"`
	RouteMatched     bool               `json:"route_matched"`
	Direct           bool               `json:"direct"`
	ClientUnchanged  bool               `json:"client_unchanged"`
	ControlPassed    bool               `json:"control_passed"`
	Tests            []TransferEvidence `json:"tests"`
}

type TransferEvidence struct {
	Nonce            string    `json:"nonce"`
	At               time.Time `json:"at"`
	Stage            string    `json:"stage"`
	UploadBytes      int64     `json:"upload_bytes"`
	DownloadBytes    int64     `json:"download_bytes"`
	UploadVerified   bool      `json:"upload_verified"`
	DownloadVerified bool      `json:"download_verified"`
	Success          bool      `json:"success"`
}

type EvidenceAssessment struct {
	Status              string   `json:"status"`
	Scope               string   `json:"scope"`
	Attempts            int      `json:"attempts"`
	SuccessfulTransfers int      `json:"successful_transfers"`
	BulkTransfers       int      `json:"bulk_transfers"`
	CanApply            bool     `json:"can_apply"`
	Blockers            []string `json:"blockers"`
}

func exactHex(s string, size int) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == size && hex.EncodeToString(b) == s
}

// AssessConnectivity intentionally rejects duplicate JSON keys and unknown
// fields. Parser errors never echo private source material. A sidecar can prove
// a sample path but cannot prove Android TUN, DNS, app import or network switching.
func AssessConnectivity(raw []byte, now time.Time) (EvidenceAssessment, error) {
	if len(raw) > 65536 {
		return EvidenceAssessment{}, ErrEvidence
	}
	if _, e := parse(raw); e != nil {
		return EvidenceAssessment{}, ErrEvidence
	}
	var v Evidence
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&v) != nil || d.Decode(new(any)) != io.EOF {
		return EvidenceAssessment{}, ErrEvidence
	}
	if v.Schema != 1 || v.StartedAt.IsZero() || v.FinishedAt.Before(v.StartedAt) ||
		v.FinishedAt.After(now) || now.Sub(v.FinishedAt) > 10*time.Minute ||
		v.FinishedAt.Sub(v.StartedAt) > 30*time.Minute || len(v.Tests) == 0 || len(v.Tests) > 128 {
		return EvidenceAssessment{}, ErrEvidence
	}
	switch v.Scope {
	case "raw_tls", "stock_sidecar", "v2rayng_tun":
	default:
		return EvidenceAssessment{}, ErrEvidence
	}
	if v.ClientVersion != "26.5.9" {
		return EvidenceAssessment{}, ErrVersion
	}
	for _, h := range []string{v.ClientCoreSHA256, v.ServerCoreSHA256, v.ConfigSHA256, v.TargetSHA256} {
		if !exactHex(h, 32) {
			return EvidenceAssessment{}, ErrEvidence
		}
	}
	a := EvidenceAssessment{Status: "NO_VERIFIED_TRANSFER", Scope: v.Scope, Attempts: len(v.Tests),
		Blockers: []string{"reported_evidence_not_device_attestation", "fleet_acceptance_and_actuator_required"}}
	seen := map[string]bool{}
	var previous time.Time
	for _, t := range v.Tests {
		if !exactHex(t.Nonce, 16) || seen[t.Nonce] || t.At.Before(v.StartedAt) || t.At.After(v.FinishedAt) ||
			(!previous.IsZero() && t.At.Before(previous)) || t.UploadBytes < 0 || t.DownloadBytes < 0 ||
			t.UploadBytes > 1<<30 || t.DownloadBytes > 1<<30 {
			return EvidenceAssessment{}, ErrEvidence
		}
		seen[t.Nonce] = true
		previous = t.At
		switch t.Stage {
		case "tcp", "tls", "proxy_auth", "payload":
		default:
			return EvidenceAssessment{}, ErrEvidence
		}
		if t.Success {
			if t.Stage != "payload" || t.UploadBytes == 0 || t.DownloadBytes == 0 || !t.UploadVerified || !t.DownloadVerified {
				return EvidenceAssessment{}, ErrEvidence
			}
			a.SuccessfulTransfers++
			if t.UploadBytes >= 16384 && t.DownloadBytes >= 65536 {
				a.BulkTransfers++
			}
		}
	}
	if !v.ControlPassed || !v.RouteMatched || v.NetworkASN == 0 || !v.Direct || !v.ClientUnchanged {
		a.Status = "UNVERIFIED_TEST_CONTEXT"
		a.Blockers = append(a.Blockers, "matching_control_route_and_unchanged_client_required")
		return a, nil
	}
	if a.SuccessfulTransfers > 0 {
		a.Status = "PARTIAL_TRANSFER"
	}
	if a.SuccessfulTransfers == a.Attempts && a.BulkTransfers >= 3 {
		a.Status = "BIDIRECTIONAL_SAMPLE"
	} else {
		a.Blockers = append(a.Blockers, "three_complete_bulk_transfers_without_failures_required")
	}
	if v.Scope == "raw_tls" {
		a.Blockers = append(a.Blockers, "proxy_protocol_and_application_path_unverified")
	} else if v.Scope == "stock_sidecar" {
		a.Blockers = append(a.Blockers, "v2rayng_import_tun_dns_and_network_switch_unverified")
	} else {
		a.Blockers = append(a.Blockers, "sustained_session_reconnect_and_per_carrier_acceptance_required")
	}
	return a, nil
}
