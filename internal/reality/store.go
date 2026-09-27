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

func (s SQLStore) Recent(
	ctx context.Context,
	panelID string,
	limitPerCandidate int,
) ([]Observation, error) {

	if s.DB == nil ||
		panelID == "" ||
		limitPerCandidate < 1 ||
		limitPerCandidate > 100 {

		return nil,
			ErrObservationStore
	}

	rows, err := s.DB.QueryContext(
		ctx,
		`
SELECT
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

observed_at

FROM (
SELECT
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

observed_at,

row_number() OVER(
PARTITION BY
target,
server_name,
port

ORDER BY
observed_at DESC
) AS rn

FROM reality_target_observations

WHERE panel_id=$1
) history

WHERE rn <= $2

ORDER BY observed_at DESC
`,
		panelID,
		limitPerCandidate,
	)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	observations :=
		make(
			[]Observation,
			0,
		)

	for rows.Next() {
		var (
			observation Observation

			medianLatency int64
		)

		err = rows.Scan(
			&observation.Candidate.Target,

			&observation.Candidate.ServerName,

			&observation.Candidate.Port,

			&observation.Reachable,

			&observation.TLSVersion,

			&observation.CertValid,

			&observation.HTTP2,

			&observation.Samples,

			&observation.Successes,

			&medianLatency,

			&observation.ObservedAt,
		)

		if err != nil {
			return nil, err
		}

		// Raw latency samples are intentionally
		// not persisted.
		//
		// The historical stability engine only
		// requires the stored per-observation
		// median.
		if observation.Successes > 0 {
			observation.LatencyMS = make([]int64, observation.Successes)
			for i := range observation.LatencyMS {
				observation.LatencyMS[i] = medianLatency
			}
		}

		observations = append(
			observations,
			observation,
		)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return observations, nil
}
