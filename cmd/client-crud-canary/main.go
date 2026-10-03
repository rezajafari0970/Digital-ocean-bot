package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/clientops"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var accountID, panelID string
	var inboundID int64
	flag.StringVar(&accountID, "account", "", "account uuid")
	flag.StringVar(&panelID, "panel", "", "panel uuid")
	flag.Int64Var(&inboundID, "inbound", 0, "inbound id")
	flag.Parse()
	if accountID == "" || panelID == "" || inboundID <= 0 {
		return fmt.Errorf("account, panel and inbound are required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	a, err := app.Bootstrap(ctx)
	if err != nil {
		return err
	}
	defer a.Close()
	j := clientops.Journal{DB: a.DB}
	clientID, err := newUUID()
	if err != nil {
		return err
	}

	off := func() {
		c, cc := context.WithTimeout(context.Background(), 5*time.Second)
		defer cc()
		_, e := a.DB.ExecContext(c, "UPDATE client_mutation_execution_gate SET enabled=false,kill_switch=true,panel_id=NULL,inbound_id=NULL,updated_at=now() WHERE singleton=true")
		if e != nil {
			log.Printf("CRITICAL: gate-off failed: %v", e)
		}
	}
	defer off()

	res, err := a.DB.ExecContext(ctx,
		"UPDATE client_mutation_execution_gate SET enabled=true,kill_switch=false,panel_id=$1,inbound_id=$2,updated_at=now() WHERE singleton=true AND enabled=false AND kill_switch=true",
		panelID, inboundID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return fmt.Errorf("gate was not safely closed before canary")
	}

	createPayload, _ := json.Marshal(map[string]any{"Client": map[string]any{
		"id": clientID, "email": "clientops-live-" + clientID[:8], "enable": true,
		"totalGB": int64(0), "expiryTime": int64(0), "limitIp": 0, "limitHwid": 0, "flow": "xtls-rprx-vision"}})
	create := clientops.Request{AccountID: accountID, PanelID: panelID, InboundID: inboundID, ClientID: clientID, Kind: clientops.KindCreate, IdempotencyKey: "crud-canary-create-" + clientID, Payload: createPayload}
	if err = runJob(ctx, j, create); err != nil {
		return fmt.Errorf("CREATE failed client=%s: %w", clientID, err)
	}

	updatePayload, _ := json.Marshal(map[string]any{"Patch": map[string]any{
		"totalGB":    int64(104857600),
		"expiryTime": time.Now().Add(30 * time.Minute).UnixMilli(),
		"limitHwid":  2,
	}})
	update := clientops.Request{AccountID: accountID, PanelID: panelID, InboundID: inboundID, ClientID: clientID, Kind: clientops.KindUpdate, IdempotencyKey: "crud-canary-update-" + clientID, Payload: updatePayload}
	if err = runJob(ctx, j, update); err != nil {
		return fmt.Errorf("UPDATE failed client=%s: %w", clientID, err)
	}

	deletePayload := json.RawMessage(`{}`)
	del := clientops.Request{AccountID: accountID, PanelID: panelID, InboundID: inboundID, ClientID: clientID, Kind: clientops.KindDelete, IdempotencyKey: "crud-canary-delete-" + clientID, Payload: deletePayload}
	if err = runJob(ctx, j, del); err != nil {
		return fmt.Errorf("DELETE failed client=%s: %w", clientID, err)
	}

	off()
	fmt.Printf("CRUD_CANARY_OK client=%s panel=%s inbound=%d\n", clientID, panelID, inboundID)
	return nil
}

func runJob(ctx context.Context, j clientops.Journal, r clientops.Request) error {
	job, _, err := j.Reserve(ctx, r)
	if err != nil {
		return err
	}
	t := time.NewTicker(250 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			cur, e := j.Get(ctx, job.ID)
			if e != nil {
				return e
			}
			switch cur.State {
			case clientops.StateSucceeded:
				return nil
			case clientops.StateFailed, clientops.StateObsolete:
				return fmt.Errorf("job %s state=%s attempts=%d error=%s", cur.ID, cur.State, cur.Attempts, cur.LastError)
			}
		}
	}
}

func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
