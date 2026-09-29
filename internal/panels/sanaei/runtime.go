package sanaei

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"
)

type RuntimeSecrets interface {
	Get(context.Context, string, string) ([]byte, error)
}

type PanelRuntime struct {
	PanelID    string
	AccountID  string
	BaseURL    string
	Client     *APIClient
	Session    *PanelSession
	mutationMu *sync.Mutex
}

func (r *PanelRuntime) WithMutation(ctx context.Context, fn func(context.Context) error) error {
	if r == nil || r.mutationMu == nil || fn == nil {
		return errors.New("sanaei runtime mutation config")
	}
	r.mutationMu.Lock()
	defer r.mutationMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn(ctx)
}

type RuntimeFactory struct {
	DB      *sql.DB
	Secrets RuntimeSecrets
	Timeout time.Duration
}

func (f RuntimeFactory) Open(ctx context.Context, panelID string) (*PanelRuntime, error) {
	if f.DB == nil || f.Secrets == nil || panelID == "" {
		return nil, errors.New("sanaei runtime config")
	}
	var accountID, baseURL, user, passwordRef string
	err := f.DB.QueryRowContext(ctx, `
SELECT pi.account_id::text,pi.base_url,x.username,x.password_secret_ref
FROM panel_instances pi
JOIN deployments d ON d.droplet_id=pi.droplet_id
JOIN xui_panel_deployments x ON x.droplet_id=pi.droplet_id
 AND x.generation=d.postinstall_generation
WHERE pi.id=$1 AND pi.enabled=true
`, panelID).Scan(&accountID, &baseURL, &user, &passwordRef)
	if err != nil {
		return nil, err
	}
	pw, err := f.Secrets.Get(ctx, accountID, passwordRef)
	if err != nil {
		return nil, err
	}
	defer func() {
		for i := range pw {
			pw[i] = 0
		}
	}()
	client, err := NewAPIClient(baseURL, Credentials{Username: user, Password: string(pw)}, nil)
	if err != nil {
		return nil, err
	}
	timeout := f.Timeout
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	client.HTTP.Timeout = timeout
	if err = client.Login(ctx); err != nil {
		return nil, err
	}
	return &PanelRuntime{PanelID: panelID, AccountID: accountID, BaseURL: baseURL, Client: client, Session: NewPanelSession(client)}, nil
}
