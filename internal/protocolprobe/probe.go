// Package protocolprobe measures datagram capability through native transport
// drivers. HTTPS reachability never authorizes UDP.
package protocolprobe

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
)

//go:embed probe.py
var Script string

const MaxCandidates = 64

type Candidate struct {
	ID       string         `json:"id"`
	Digest   string         `json:"digest"`
	Outbound map[string]any `json:"outbound"`
}
type Observation struct {
	ID     string `json:"id"`
	Digest string `json:"digest"`
	DNS    bool   `json:"dns"`
	QUIC   bool   `json:"quic"`
	Code   string `json:"code"`
}
type Request struct {
	Candidates []Candidate `json:"candidates"`
	CoreHashes []string    `json:"core_hashes"`
}

func Fingerprint(outbound map[string]any) string {
	// A program change invalidates old evidence as well as endpoint, credentials,
	// TLS trust or XUDP changes. Digests are not a substitute for secret storage.
	raw, _ := json.Marshal(outbound)
	h := sha256.New()
	h.Write([]byte(Script))
	h.Write([]byte{0})
	h.Write(raw)
	return hex.EncodeToString(h.Sum(nil))
}
func Decode(raw string, candidates []Candidate) ([]Observation, error) {
	bad := errors.New("protocol probe result invalid")
	if len(raw) > 65536 || len(candidates) < 1 || len(candidates) > MaxCandidates {
		return nil, bad
	}
	var result struct {
		Results []Observation `json:"results"`
		Error   string        `json:"error"`
	}
	if json.Unmarshal([]byte(raw), &result) != nil || result.Error != "" || len(result.Results) != len(candidates) {
		return nil, bad
	}
	seen := map[string]bool{}
	for i, o := range result.Results {
		c := candidates[i]
		if c.ID == "" || seen[c.ID] || len(c.Digest) != 64 || o.ID != c.ID || o.Digest != c.Digest {
			return nil, bad
		}
		seen[c.ID] = true
		switch o.Code {
		case "ok":
			if !o.DNS || !o.QUIC {
				return nil, bad
			}
		case "datagram_unverified":
			if o.DNS && o.QUIC {
				return nil, bad
			}
		default:
			return nil, bad
		}
	}
	return result.Results, nil
}
