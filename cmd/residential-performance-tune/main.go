// residential-performance-tune is an operator-only client of the supported
// Store. It requires the same protected service environment as other commands;
// it never writes routing assignments directly or calls panel mutation APIs.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/residentialperf"
	"io"
	"os"
	"syscall"
	"time"
)

func main() {
	execute := flag.Bool("execute", false, "execute one exact stdin request; default is read-only status")
	reconcile := flag.Bool("reconcile", false, "read-only verification of exact stdin request against committed operation")
	flag.Parse()
	if e := run(*execute, *reconcile); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run(execute, reconcile bool) error {
	if execute && reconcile {
		return errors.New("execute and reconcile are mutually exclusive")
	}
	// Deployments hold this lock exclusively from staging through rollback. API
	// writers are stopped before downgrade checks; operator commands share the lock.
	if execute {
		lock, e := os.OpenFile("/opt/.digital-ocean-bot-deploy.lock", os.O_CREATE|os.O_RDWR, 0600)
		if e != nil {
			return errors.New("deployment lock unavailable")
		}
		defer lock.Close()
		if e = syscall.Flock(int(lock.Fd()), syscall.LOCK_SH|syscall.LOCK_NB); e != nil {
			return errors.New("deployment in progress; no tuning operation executed")
		}
		defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var q residentialperf.Request
	if execute || reconcile {
		dec := json.NewDecoder(io.LimitReader(os.Stdin, 32768))
		dec.DisallowUnknownFields()
		if e := dec.Decode(&q); e != nil {
			return errors.New("invalid bounded JSON request")
		}
		if e := dec.Decode(new(any)); e != io.EOF {
			return errors.New("exactly one JSON request required")
		}
		switch q.Action {
		case "tune_start", "tune_cancel", "tune_publish", "tune_restore":
		default:
			return errors.New("only supported tuning actions are allowed")
		}
		// Selection tuning retains its reviewed bounds. Admission requires trusted evidence;
		// Store enforces an otherwise identical parent, one panel and mandatory expiry.
		if execute && q.Action == "tune_start" && (q.Config == nil || (q.AdmissionEvidenceID == "" || len(q.Config.ExcludedProxyIDs) == 0) && (q.Config.FastCount != 3 || q.Config.FastShare != 90)) {
			return errors.New("reviewed candidate requires fast_count=3 and fast_share=90")
		}
	}
	if reconcile {
		if os.Getenv("DATABASE_URL") == "" {
			return errors.New("protected database environment required")
		}
		db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
		if err != nil {
			return errors.New("read-only database unavailable")
		}
		defer db.Close()
		receipt, err := (residentialperf.Store{DB: db}).Reconcile(ctx, q)
		if err != nil {
			return errors.New("exact request reconciliation failed")
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"found": receipt != nil, "receipt": receipt, "exact_request_verified": receipt != nil, "evidence_id": q.AdmissionEvidenceID})
	}
	a, e := app.Bootstrap(ctx)
	if e != nil {
		return errors.New("service bootstrap unavailable")
	}
	defer a.Close()

	var receipt *residentialperf.Receipt
	if execute {
		r, e := (residentialperf.Store{DB: a.DB}).Do(ctx, q)
		if e != nil {
			var known *residentialperf.Error
			if errors.As(e, &known) {
				return known
			}
			return errors.New("operation outcome unconfirmed; inspect status and replay identical request_id")
		}
		receipt = &r
	}
	var current []byte
	e = a.DB.QueryRowContext(ctx, `SELECT json_build_object('id',e.id,'state',e.state,'version',e.version,
 'mode',e.duration_mode,'scope',e.publish_scope,'config',e.spec,'active_tuning',e.tuning,
 'targets',(SELECT json_build_object('total',count(*),'pending',count(*) FILTER(WHERE
 EXISTS(SELECT 1 FROM panel_instances pi JOIN droplets d ON d.id=pi.droplet_id WHERE pi.id=t.panel_id AND d.state<>'DELETED')
 AND NOT COALESCE(p.experiment_id=t.experiment_id AND p.generation=t.generation AND p.applied_generation=t.generation
 AND p.verified_at>clock_timestamp()-interval '60 seconds' AND r.state='APPLIED' AND r.revision=c.revision
 AND r.performance_generation=t.generation AND r.verified_at>clock_timestamp()-interval '60 seconds',false)))
 FROM residential_performance_targets t LEFT JOIN residential_performance_panels p ON p.panel_id=t.panel_id
 LEFT JOIN panel_routing_state r ON r.panel_id=t.panel_id CROSS JOIN residential_routing_control c WHERE t.experiment_id=e.id))
 FROM residential_performance_experiments e ORDER BY created_at DESC LIMIT 1`).Scan(&current)
	if e != nil {
		return errors.New("current status unavailable; receipt may already be committed")
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"receipt": receipt, "current": json.RawMessage(current), "recovery_contract": "Latest only: a new trial replaces the preceding completed publication recovery point; save its exact spec first."})
}
