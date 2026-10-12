package app

import (
	"context"
	"fmt"
)

type PanelSettings struct {
	Port    int
	WebPath string
}

func (a *Application) PanelSettings(ctx context.Context) (PanelSettings, error) {
	x := PanelSettings{Port: 18080, WebPath: "/admin"}
	err := a.DB.QueryRowContext(ctx, `SELECT listen_port,web_path FROM panel_settings WHERE id=true`).Scan(&x.Port, &x.WebPath)
	return x, err
}
func (x PanelSettings) Addr() string { return fmt.Sprintf("0.0.0.0:%d", x.Port) }
