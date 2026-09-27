package credentials

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var ErrRegistry = errors.New(
	"reality credential registry failure",
)

type SecretWriter interface {
	Put(
		context.Context,
		string,
		string,
		string,
		[]byte,
	) error
}

type KeyGenerator interface {
	Generate(
		context.Context,
	) (KeyPair, error)
}

type Record struct {
	PanelID string

	ManagedKey string

	UUIDSecretRef string

	PrivateKeySecretRef string

	PublicKey string

	ShortID string
}

type Registry struct {
	DB *sql.DB

	Secrets SecretWriter

	Keys KeyGenerator
}

func (r Registry) Ensure(
	ctx context.Context,
	accountID string,
	panelID string,
	managedKey string,
) (Record, error) {

	if r.DB == nil ||
		r.Secrets == nil ||
		r.Keys == nil ||
		accountID == "" ||
		panelID == "" ||
		managedKey == "" {

		return Record{},
			ErrRegistry
	}

	tx, err := r.DB.BeginTx(
		ctx,
		nil,
	)

	if err != nil {
		return Record{}, err
	}

	defer tx.Rollback()

	// Serialize Ensure() globally across workers and
	// processes for this exact credential identity.
	if err = credentialLock(
		ctx,
		tx,
		panelID,
		managedKey,
	); err != nil {

		return Record{}, err
	}

	existing, err :=
		getRecord(
			ctx,
			tx,
			panelID,
			managedKey,
		)

	switch {

	case err == nil:

		if err = tx.Commit(); err != nil {
			return Record{}, err
		}

		return existing, nil

	case !errors.Is(
		err,
		sql.ErrNoRows,
	):

		return Record{}, err
	}

	uuid, err := UUID()

	if err != nil {
		return Record{}, err
	}

	shortID, err :=
		ShortID(
			4,
		)

	if err != nil {
		return Record{}, err
	}

	keyPair, err :=
		r.Keys.Generate(
			ctx,
		)

	if err != nil {
		return Record{}, err
	}

	defer Wipe(
		keyPair.Private,
	)

	uuidRef :=
		fmt.Sprintf(
			"reality-uuid:%s:%s",
			panelID,
			managedKey,
		)

	privateKeyRef :=
		fmt.Sprintf(
			"reality-private:%s:%s",
			panelID,
			managedKey,
		)

	uuidBytes :=
		[]byte(
			uuid,
		)

	defer Wipe(
		uuidBytes,
	)

	if err = r.Secrets.Put(
		ctx,
		accountID,
		uuidRef,
		"reality_client_uuid",
		uuidBytes,
	); err != nil {

		return Record{}, err
	}

	if err = r.Secrets.Put(
		ctx,
		accountID,
		privateKeyRef,
		"reality_private_key",
		keyPair.Private,
	); err != nil {

		return Record{}, err
	}

	_, err = tx.ExecContext(
		ctx,
		`
INSERT INTO reality_credentials(
panel_id,
managed_key,
uuid_secret_ref,
private_key_secret_ref,
public_key,
short_id
)
VALUES(
$1,
$2,
$3,
$4,
$5,
$6
)
`,
		panelID,
		managedKey,
		uuidRef,
		privateKeyRef,
		keyPair.Public,
		shortID,
	)

	if err != nil {
		return Record{}, err
	}

	record, err :=
		getRecord(
			ctx,
			tx,
			panelID,
			managedKey,
		)

	if err != nil {
		return Record{}, err
	}

	if err = tx.Commit(); err != nil {
		return Record{}, err
	}

	return record, nil
}

type queryRower interface {
	QueryRowContext(
		context.Context,
		string,
		...any,
	) *sql.Row
}

func getRecord(
	ctx context.Context,
	query queryRower,
	panelID string,
	managedKey string,
) (Record, error) {

	var record Record

	err := query.QueryRowContext(
		ctx,
		`
SELECT
panel_id::text,
managed_key,
uuid_secret_ref,
private_key_secret_ref,
public_key,
short_id

FROM reality_credentials

WHERE panel_id=$1
  AND managed_key=$2
`,
		panelID,
		managedKey,
	).Scan(
		&record.PanelID,
		&record.ManagedKey,
		&record.UUIDSecretRef,
		&record.PrivateKeySecretRef,
		&record.PublicKey,
		&record.ShortID,
	)

	return record, err
}

func (r Registry) get(
	ctx context.Context,
	panelID string,
	managedKey string,
) (Record, error) {

	return getRecord(
		ctx,
		r.DB,
		panelID,
		managedKey,
	)
}
