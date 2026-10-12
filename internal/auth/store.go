package auth

import (
	"context"
	"database/sql"
	"errors"
	"golang.org/x/crypto/bcrypt"
)

type SQLStore struct{ DB *sql.DB }

func (s SQLStore) FindUser(ctx context.Context, username string) (User, error) {
	var u User
	err := s.DB.QueryRowContext(ctx, `SELECT id::text,username,password_hash,role,enabled,must_change_password,bootstrap_token_hash FROM admin_users WHERE username=$1`, username).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.Enabled, &u.MustChangePassword, &u.BootstrapTokenHash)
	return u, err
}
func (s SQLStore) CreateSession(ctx context.Context, x Session) error {
	// The shared row lock closes login-versus-password-change races.
	result, err := s.DB.ExecContext(ctx, `INSERT INTO admin_sessions(id,user_id,token_hash,expires_at,created_at,last_seen_at)
 SELECT gen_random_uuid(),u.id,$2,$3,now(),now()
 FROM (SELECT id FROM admin_users WHERE id=$1 AND password_hash=$4 AND username=$5 AND enabled FOR SHARE) u`,
		x.UserID, x.TokenHash, x.ExpiresAt, x.CredentialHash, x.CredentialUsername)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrInvalidCredentials
	}
	return nil
}
func (s SQLStore) FindSession(ctx context.Context, hash []byte) (Session, User, error) {
	var x Session
	var u User
	err := s.DB.QueryRowContext(ctx, `SELECT s.id::text,s.user_id::text,s.token_hash,s.expires_at,s.created_at,s.last_seen_at,u.id::text,u.username,u.password_hash,u.role,u.enabled,u.must_change_password,u.bootstrap_token_hash FROM admin_sessions s JOIN admin_users u ON u.id=s.user_id WHERE s.token_hash=$1`, hash).Scan(&x.ID, &x.UserID, &x.TokenHash, &x.ExpiresAt, &x.CreatedAt, &x.LastSeenAt, &u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.Enabled, &u.MustChangePassword, &u.BootstrapTokenHash)
	if err == nil {
		_, _ = s.DB.ExecContext(ctx, `UPDATE admin_sessions SET last_seen_at=now() WHERE id=$1`, x.ID)
	}
	return x, u, err
}
func (s SQLStore) DeleteSession(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM admin_sessions WHERE id=$1`, id)
	return err
}

func (s SQLStore) ChangePassword(ctx context.Context, userID, current, next string) error {
	hash, err := HashPassword(next)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var prior string
	err = tx.QueryRowContext(ctx, `SELECT password_hash FROM admin_users WHERE id=$1 AND enabled FOR UPDATE`, userID).Scan(&prior)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInvalidCredentials
	}
	if err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(prior), []byte(current)) != nil || current == next {
		return ErrInvalidCredentials
	}
	if _, err = tx.ExecContext(ctx, `UPDATE admin_users SET password_hash=$2,must_change_password=false,bootstrap_token_hash=NULL,updated_at=now() WHERE id=$1`, userID, hash); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM admin_sessions WHERE user_id=$1`, userID); err != nil {
		return err
	}
	return tx.Commit()
}
