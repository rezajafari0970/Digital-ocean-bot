package worker

import (
	"context"
	"sync"
)

// Dispatcher keeps slow work in bounded lanes across polling rounds. Identity
// deduplication complements (and never replaces) the durable mutation leases.
type Dispatcher struct {
	mu                sync.Mutex
	active            map[string]string
	accounts          map[string]int
	limit, perAccount int
	wg                sync.WaitGroup
}

func NewDispatcher(limit, perAccount int) *Dispatcher {
	if limit < 1 {
		limit = 1
	}
	if perAccount < 1 {
		perAccount = 1
	}
	return &Dispatcher{active: map[string]string{}, accounts: map[string]int{}, limit: limit, perAccount: perAccount}
}
func (d *Dispatcher) Submit(ctx context.Context, key, account string, fn func()) bool {
	d.mu.Lock()
	if ctx.Err() != nil || len(d.active) >= d.limit || d.accounts[account] >= d.perAccount {
		d.mu.Unlock()
		return false
	}
	if _, ok := d.active[key]; ok {
		d.mu.Unlock()
		return false
	}
	d.active[key] = account
	d.accounts[account]++
	d.wg.Add(1)
	d.mu.Unlock()
	go func() {
		defer d.wg.Done()
		defer func() { d.mu.Lock(); delete(d.active, key); d.accounts[account]--; d.mu.Unlock() }()
		fn()
	}()
	return true
}
func (d *Dispatcher) Wait() { d.wg.Wait() }
