package auth

import (
	"context"
	"database/sql"
)

type SQLStore struct{ DB *sql.DB }

func (s SQLStore) FindUser(ctx context.Context, username string) (User, error) {
	var u User
	err := s.DB.QueryRowContext(ctx, `SELECT id::text,username,password_hash,role,enabled FROM admin_users WHERE username=$1`, username).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.Enabled)
	return u, err
}
func (s SQLStore) CreateSession(ctx context.Context, x Session) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO admin_sessions(id,user_id,token_hash,expires_at,created_at,last_seen_at) VALUES(gen_random_uuid(),$1,$2,$3,now(),now())`, x.UserID, x.TokenHash, x.ExpiresAt)
	return err
}
func (s SQLStore) FindSession(ctx context.Context, hash []byte) (Session, User, error) {
	var x Session
	var u User
	err := s.DB.QueryRowContext(ctx, `SELECT s.id::text,s.user_id::text,s.token_hash,s.expires_at,s.created_at,s.last_seen_at,u.id::text,u.username,u.password_hash,u.role,u.enabled FROM admin_sessions s JOIN admin_users u ON u.id=s.user_id WHERE s.token_hash=$1`, hash).Scan(&x.ID, &x.UserID, &x.TokenHash, &x.ExpiresAt, &x.CreatedAt, &x.LastSeenAt, &u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.Enabled)
	if err == nil {
		_, _ = s.DB.ExecContext(ctx, `UPDATE admin_sessions SET last_seen_at=now() WHERE id=$1`, x.ID)
	}
	return x, u, err
}
func (s SQLStore) DeleteSession(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM admin_sessions WHERE id=$1`, id)
	return err
}
