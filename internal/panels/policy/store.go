package policy

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

var ErrInvalidPolicy = errors.New("invalid inbound policy")

type SQLStore struct {
	DB *sql.DB
}

func (s SQLStore) Put(
	ctx context.Context,
	p InboundPolicy,
) (InboundPolicy, error) {

	if s.DB == nil {
		return InboundPolicy{}, ErrInvalidPolicy
	}
	if err := Validate(p); err != nil {
		return InboundPolicy{}, err
	}

	preferredPorts, err :=
		json.Marshal(
			p.PreferredPorts,
		)

	if err != nil {
		return InboundPolicy{}, err
	}

	reservedPorts, err :=
		json.Marshal(
			p.ReservedPorts,
		)

	if err != nil {
		return InboundPolicy{}, err
	}

	_, err = s.DB.ExecContext(
		ctx,
		`
INSERT INTO panel_inbound_policies(
panel_id,
policy_key,

enabled,

desired_count,

protocol,
transport,
security,
listen,

preferred_ports,

dynamic_port_start,
dynamic_port_end,

reserved_ports,

clients_per_inbound,

allow_delete
)
VALUES(
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

$14
)

ON CONFLICT(
panel_id,
policy_key
)

DO UPDATE SET
revision=
panel_inbound_policies.revision + 1,

enabled=
excluded.enabled,

desired_count=
excluded.desired_count,

protocol=
excluded.protocol,

transport=
excluded.transport,

security=
excluded.security,

listen=
excluded.listen,

preferred_ports=
excluded.preferred_ports,

dynamic_port_start=
excluded.dynamic_port_start,

dynamic_port_end=
excluded.dynamic_port_end,

reserved_ports=
excluded.reserved_ports,

clients_per_inbound=
excluded.clients_per_inbound,

allow_delete=
excluded.allow_delete,

updated_at=now()
`,
		p.PanelID,
		p.ID,

		p.Enabled,

		p.DesiredCount,

		p.Protocol,
		p.Transport,
		p.Security,
		p.Listen,

		preferredPorts,

		p.DynamicPortStart,
		p.DynamicPortEnd,

		reservedPorts,

		p.ClientsPerInbound,

		p.AllowDelete,
	)

	if err != nil {
		return InboundPolicy{}, err
	}

	return p, nil
}
