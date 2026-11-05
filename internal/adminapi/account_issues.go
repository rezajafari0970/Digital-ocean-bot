package adminapi

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/capacity"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/redact"
)

// Independent evidence stays visible even when another restriction wins the
// scheduler's admission decision. These diagnostics never authorize mutations.
type accountIssue struct {
	Kind       string     `json:"kind"`
	Code       string     `json:"code"`
	Detail     string     `json:"detail"`
	ObservedAt *time.Time `json:"observed_at,omitempty"`
}

var diagnosticURL = regexp.MustCompile(`(?i)\b(?:https?|socks5?h?)://[^\s]+`)

func accountDiagnosticDetail(detail, fallback string) string {
	detail = strings.TrimSpace(detail)
	if strings.HasPrefix(detail, "{") || strings.HasPrefix(detail, "[") {
		// Worker receipts are structured; never render their arbitrary metadata.
		var receipt struct {
			ProviderError string `json:"provider_error"`
		}
		if json.Unmarshal([]byte(detail), &receipt) == nil {
			detail = receipt.ProviderError
		} else {
			detail = ""
		}
	}
	if strings.TrimSpace(detail) == "" {
		detail = fallback
	}
	detail = redact.Text(diagnosticURL.ReplaceAllString(detail, "[connection address redacted]"))
	detail = strings.Join(strings.Fields(detail), " ")
	runes := []rune(detail)
	if len(runes) > 800 {
		detail = string(runes[:800]) + "…"
	}
	return detail
}

func issueTime(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func (x *accountBuildState) addAccountIssues(state, detail, observation, observationDetail string, stateAt, errorAt sql.NullTime, unresolved int) {
	x.Issues = make([]accountIssue, 0)
	if state != "" && state != "ACTIVE" {
		x.Issues = append(x.Issues, accountIssue{"provider", state, accountDiagnosticDetail(detail, strings.ReplaceAll(state, "_", " ")), issueTime(stateAt)})
	} else if x.State == "LOCKED" || x.State == "PROVIDER_WARNING" || x.State == "TRIAL_RESTRICTED" {
		x.Issues = append(x.Issues, accountIssue{"provider", x.State, accountDiagnosticDetail(x.Reason, x.State), nil})
	}
	if observation != "" {
		x.Issues = append(x.Issues, accountIssue{"observation", observation, accountDiagnosticDetail(observationDetail, strings.ReplaceAll(observation, "_", " ")), issueTime(errorAt)})
	}
	if x.Block != nil {
		x.Issues = append(x.Issues, accountIssue{"creation", x.Block.Code, capacity.CreateBlockReason(x.Block.Code), nil})
	}
	if unresolved > 0 {
		x.Issues = append(x.Issues, accountIssue{"creation", "CREATE_OUTCOME_UNKNOWN", fmt.Sprintf("%d create request(s) still need a verified provider result. Reservations remain held; no duplicate create will be sent. Check Build Activity and the provider task history.", unresolved), nil})
	}
}
