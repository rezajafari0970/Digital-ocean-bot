package serverprotection

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/supervision"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type pollSecrets struct{}

func (pollSecrets) Get(context.Context, string, string) ([]byte, error) {
	return []byte("isolated-fixture-key"), nil
}

type pollSSH struct {
	db                        *sql.DB
	hash                      string
	cleanup                   chan struct{}
	entered                   chan string
	blockPanel                string
	failure                   error
	mu                        sync.Mutex
	active, peak              map[bool]int
	total, peakTotal, uploads int
	calls                     []string
}

func newPollController(t *testing.T, db *sql.DB) (Controller, *pollSSH) {
	t.Helper()
	dir := t.TempDir()
	data := []byte("isolated guardian artifact")
	if e := os.WriteFile(filepath.Join(dir, "server-guardian-linux-amd64"), data, 0600); e != nil {
		t.Fatal(e)
	}
	ssh := &pollSSH{db: db, hash: fmt.Sprintf("%x", sha256.Sum256(data)), entered: make(chan string, 1000), active: map[bool]int{}, peak: map[bool]int{}}
	return Controller{DB: db, Secrets: pollSecrets{}, SSH: ssh, ArtifactDir: dir}, ssh
}
func (s *pollSSH) Upload(context.Context, provisioning.Target, []byte, string, string, os.FileMode) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.uploads++
	return errors.New("unexpected fixture upload")
}
func (s *pollSSH) Run(ctx context.Context, t provisioning.Target, key []byte, cmd string) (string, error) {
	if s.failure != nil {
		return "", s.failure
	}
	if strings.Contains(cmd, "uname -m") {
		return "x86_64\n" + s.hash + "\n", nil
	}
	var revision int64
	var enabled bool
	if e := s.db.QueryRowContext(ctx, "SELECT desired_revision,desired_enabled FROM server_protection_nodes WHERE panel_id=$1", t.DropletID).Scan(&revision, &enabled); e != nil {
		return "", e
	}
	s.mu.Lock()
	s.calls = append(s.calls, t.DropletID)
	s.active[enabled]++
	s.total++
	if s.active[enabled] > s.peak[enabled] {
		s.peak[enabled] = s.active[enabled]
	}
	if s.total > s.peakTotal {
		s.peakTotal = s.total
	}
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.active[enabled]--; s.total--; s.mu.Unlock() }()
	s.entered <- t.DropletID
	if s.cleanup != nil && (!enabled || s.blockPanel == t.DropletID) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-s.cleanup:
		}
	}
	st := Status{Version: Version, Revision: revision, Enabled: enabled, AgentRunning: enabled, NFTSupported: true, Ports: []int{443}, State: "READY", XUIState: "active", ObservedAt: time.Now()}
	if !enabled {
		st.State = "DISABLED"
	}
	raw, e := json.Marshal(st)
	return string(raw), e
}
func waitPoll(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("bounded polling condition not reached")
}
func TestProtectionCleanupCannotHoldEnabledReceiptsOrNextCycle(t *testing.T) {
	db := pollingDB(t)
	for i := 1; i <= 70; i++ {
		pollingSeed(t, db, i, false)
	}
	for i := 101; i <= 108; i++ {
		pollingSeed(t, db, i, true)
	}
	c, ssh := newPollController(t, db)
	ssh.cleanup = make(chan struct{})
	budget := worker.NewWorkBudget(10)
	c.Admit = budget.Do
	ctx, cancel := context.WithCancel(context.Background())
	reg := supervision.New(nil)
	policy := supervision.Policy{Loop: 2 * time.Minute, Work: 45 * time.Second, Idle: 5 * time.Second}
	enabledCtx, e := reg.Register(ctx, "server-protection", policy)
	if e != nil {
		t.Fatal(e)
	}
	cleanupCtx, e := reg.Register(ctx, "server-protection-cleanup", policy)
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	defer func() {
		cancel()
		done := make(chan struct{})
		go func() { wg.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(6 * time.Second):
			t.Error("lane shutdown did not join")
		}
		if budget.Snapshot()["active"] != 0 {
			t.Error("work slots leaked")
		}
		if db.Stats().InUse != 0 {
			t.Error("SQL sessions leaked")
		}
	}()
	wg.Add(1)
	go func() { defer wg.Done(); c.RunCleanup(cleanupCtx) }()
	waitPoll(t, 2*time.Second, func() bool { ssh.mu.Lock(); defer ssh.mu.Unlock(); return ssh.active[false] == 2 })
	wg.Add(1)
	go func() { defer wg.Done(); c.Run(enabledCtx) }()
	var first time.Time
	waitPoll(t, 3*time.Second, func() bool {
		var n int
		e := db.QueryRow("SELECT count(*),coalesce(min(verified_at),now()) FROM server_protection_nodes WHERE desired_enabled AND state='APPLIED'").Scan(&n, &first)
		return e == nil && n == 8
	})
	pollingExec(t, db, "UPDATE server_protection_nodes SET next_check_at=now() WHERE desired_enabled")
	waitPoll(t, 7*time.Second, func() bool {
		var n int
		return db.QueryRow("SELECT count(*) FROM server_protection_nodes WHERE desired_enabled AND verified_at>$1", first.Add(time.Second)).Scan(&n) == nil && n == 8
	})
	if e := reg.Check(); e != nil {
		t.Fatal("independent supervision", e)
	}
	ssh.mu.Lock()
	defer ssh.mu.Unlock()
	if ssh.active[false] != 2 || ssh.peak[false] > 2 || ssh.peak[true] > 6 || ssh.peakTotal > 8 {
		t.Fatalf("lane ceilings: cleanup=%d enabled=%d combined=%d", ssh.peak[false], ssh.peak[true], ssh.peakTotal)
	}
}
func TestProtectionSelectionBeforeLimitAndOldestDueFairness(t *testing.T) {
	db := pollingDB(t)
	for i := 1; i <= 70; i++ {
		pollingSeed(t, db, i, false)
	}
	for i := 101; i <= 103; i++ {
		pollingSeed(t, db, i, true)
	}
	future := pollingSeed(t, db, 104, true)
	deleted := pollingSeed(t, db, 200, false)
	pollingExec(t, db, "UPDATE server_protection_nodes SET next_check_at='2000-01-01'::timestamptz,state='ERROR'")
	pollingExec(t, db, "UPDATE server_protection_nodes SET next_check_at=now()+interval '1 hour' WHERE panel_id=$1", future)
	pollingExec(t, db, "UPDATE droplets SET state='DELETED' WHERE id=$1", deleted)
	c, ssh := newPollController(t, db)
	yes, no := true, false
	if e := c.round(context.Background(), &yes, 6); e != nil {
		t.Fatal(e)
	}
	if len(ssh.calls) != 3 {
		t.Fatalf("enabled selection lost behind cleanup LIMIT: %d", len(ssh.calls))
	}
	ssh.calls = nil
	if e := c.round(context.Background(), &no, 2); e != nil {
		t.Fatal(e)
	}
	if len(ssh.calls) != 64 {
		t.Fatalf("cleanup batch=%d", len(ssh.calls))
	}
	for _, id := range ssh.calls {
		if id > "10000000-0000-4000-8000-000000000064" {
			t.Fatal("deterministic oldest tie displaced", id)
		}
	}
	newID := pollingSeed(t, db, 300, false)
	pollingExec(t, db, "UPDATE server_protection_nodes SET next_check_at=now(),state='PENDING' WHERE panel_id=$1", newID)
	ssh.calls = nil
	if e := c.round(context.Background(), &no, 2); e != nil {
		t.Fatal(e)
	}
	if len(ssh.calls) != 7 {
		t.Fatalf("old retries or new policy skipped: %d", len(ssh.calls))
	}
	for _, id := range ssh.calls[:2] {
		if id == newID {
			t.Fatal("new arrival displaced oldest retry")
		}
	}
}
func pollTarget(id string, enabled bool, revision int64) nodeTarget {
	return nodeTarget{Panel: id, Droplet: id, Account: "00000000-0000-4000-8000-000000000001", Host: id, User: "root", KeyRef: "fixture", Policy: Policy{PanelID: id, Enabled: enabled, Revision: revision}}
}
func TestProtectionCompetingOwnerStaleRevisionAndStickyReceipt(t *testing.T) {
	db := pollingDB(t)
	id := pollingSeed(t, db, 1, true)
	pollingExec(t, db, "UPDATE server_protection_nodes SET verified_status='{\"admission_blocked\":true,\"enabled\":true}',verified_at=now() WHERE panel_id=$1", id)
	c, ssh := newPollController(t, db)
	ssh.cleanup = make(chan struct{})
	ssh.blockPanel = id
	target := pollTarget(id, true, 1)
	done := make(chan struct{})
	go func() { defer close(done); c.reconcile(context.Background(), target) }()
	select {
	case <-ssh.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("first owner absent")
	}
	c.reconcile(context.Background(), target)
	ssh.mu.Lock()
	n := len(ssh.calls)
	ssh.mu.Unlock()
	if n != 1 {
		t.Fatal("parallel reconciliation entered SSH")
	}
	pollingExec(t, db, "UPDATE server_protection_nodes SET desired_revision=2,desired_enabled=false,state='PENDING',next_check_at=now() WHERE panel_id=$1", id)
	close(ssh.cleanup)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("old owner not joined")
	}
	var state, blocked string
	var due bool
	if e := db.QueryRow("SELECT state,verified_status->>'admission_blocked',next_check_at<=now() FROM server_protection_nodes WHERE panel_id=$1", id).Scan(&state, &blocked, &due); e != nil || state != "PENDING" || blocked != "true" || !due {
		t.Fatal("stale receipt altered new policy or verified admission", state, blocked, due, e)
	}
	c.reconcile(context.Background(), target)
	if len(ssh.calls) != 1 {
		t.Fatal("obsolete preflight opened SSH")
	}
	c.reconcile(context.Background(), pollTarget(id, false, 2))
	if e := db.QueryRow("SELECT state,verified_status->>'admission_blocked' FROM server_protection_nodes WHERE panel_id=$1", id).Scan(&state, &blocked); e != nil || state != "DISABLED" || blocked != "false" {
		t.Fatal("current cleanup failed", state, blocked, e)
	}
	// Same-revision selected work can run again sequentially, as before this change; never concurrently.
	c.reconcile(context.Background(), pollTarget(id, false, 2))
	if len(ssh.calls) != 3 {
		t.Fatal("documented one-shot compatibility changed")
	}
}
func TestProtectionCanceledAdmissionAndHostKeyFailurePreserveEvidence(t *testing.T) {
	db := pollingDB(t)
	id := pollingSeed(t, db, 1, true)
	pollingExec(t, db, "UPDATE server_protection_nodes SET verified_status='{\"admission_blocked\":true,\"enabled\":true}',verified_at=now() WHERE panel_id=$1", id)
	c, ssh := newPollController(t, db)
	ssh.failure = errors.New("ssh host key mismatch")
	c.reconcile(context.Background(), pollTarget(id, true, 1))
	var blocked, state string
	if e := db.QueryRow("SELECT state,verified_status->>'admission_blocked' FROM server_protection_nodes WHERE panel_id=$1", id).Scan(&state, &blocked); e != nil || state != "ERROR" || blocked != "true" || ssh.uploads != 0 {
		t.Fatal("host key failure weakened evidence", state, blocked, e)
	}
	budget := worker.NewWorkBudget(1)
	held := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = budget.Do(context.Background(), func(context.Context) error { close(held); <-release; return nil })
	}()
	<-held
	c.Admit = budget.Do
	pollingExec(t, db, "UPDATE server_protection_nodes SET next_check_at=now()")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	yes := true
	_ = c.round(ctx, &yes, 6)
	if budget.Snapshot()["active"] != 1 || budget.Snapshot()["waiting"] != 0 {
		t.Fatal("queued cancellation released someone else's slot")
	}
	close(release)
	<-done
	if len(ssh.calls) != 0 {
		t.Fatal("canceled/host-key-failed work entered remote mutation")
	}
}
