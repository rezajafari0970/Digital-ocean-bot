package reality

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrObservationStore = errors.New(
	"invalid reality observation store request",
)

type SQLStore struct {
	DB *sql.DB
}

func (s SQLStore) Save(
	ctx context.Context,
	panelID string,
	observation Observation,
) error {

	if s.DB == nil ||
		panelID == "" ||
		observation.ObservedAt.IsZero() {

		return ErrObservationStore
	}

	if err := ValidateCandidate(
		observation.Candidate,
	); err != nil {
		return err
	}
	if observation.Samples < 0 || observation.Successes < 0 || observation.Successes > observation.Samples || len(observation.LatencyMS) != observation.Successes {
		return ErrObservationStore
	}

	score := Evaluate(
		observation,
	)

	_, err := s.DB.ExecContext(
		ctx,
		`
INSERT INTO reality_target_observations(
id,

panel_id,

target,
server_name,
port,

reachable,

tls_version,

cert_valid,

http2,

samples,
successes,

median_latency_ms,

success_ratio,

eligible,

score,

reason,

observed_at
)
VALUES(
gen_random_uuid(),

$1,

$2,
$3,
$4,

$5,

$6,

$7,

$8,

$9,
$10,

$11,

$12,

$13,

$14,

$15,

$16
)
`,
		panelID,

		observation.Candidate.Target,
		observation.Candidate.ServerName,
		observation.Candidate.Port,

		observation.Reachable,

		observation.TLSVersion,

		observation.CertValid,

		observation.HTTP2,

		observation.Samples,
		observation.Successes,

		score.MedianLatencyMS,

		score.SuccessRatio,

		score.Eligible,

		score.Value,

		score.Reason,

		observation.ObservedAt,
	)

	return err
}

func (s SQLStore) Select(
	ctx context.Context,
	panelID string,
	score Score,
	selectedAt time.Time,
) error {

	if s.DB == nil ||
		panelID == "" ||
		selectedAt.IsZero() ||
		!score.Eligible {

		return ErrObservationStore
	}

	if err := ValidateCandidate(
		score.Candidate,
	); err != nil {

		return err
	}

	_, err := s.DB.ExecContext(
		ctx,
		`
INSERT INTO reality_target_selections(
panel_id,

target,
server_name,
port,

score,

selected_at
)
VALUES(
$1,

$2,
$3,
$4,

$5,

$6
)

ON CONFLICT(
panel_id
)

DO UPDATE SET
target=
excluded.target,

server_name=
excluded.server_name,

port=
excluded.port,

score=
excluded.score,

selected_at=
excluded.selected_at,

updated_at=
now()
`,
		panelID,

		score.Candidate.Target,
		score.Candidate.ServerName,
		score.Candidate.Port,

		score.Value,

		selectedAt,
	)

	return err
}
