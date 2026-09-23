package app

import (
	"context"
	"fmt"
)

type PanelSettings struct {
	Host    string
	Port    int
	WebPath string
}

func (a *Application) PanelSettings(ctx context.Context) (PanelSettings, error) {
	x := PanelSettings{Host: "127.0.0.1", Port: 18080, WebPath: "/admin"}
	err := a.DB.QueryRowContext(ctx, `SELECT listen_host,listen_port,web_path FROM panel_settings WHERE id=true`).Scan(&x.Host, &x.Port, &x.WebPath)
	return x, err
}
func (x PanelSettings) Addr() string { return fmt.Sprintf("%s:%d", x.Host, x.Port) }
