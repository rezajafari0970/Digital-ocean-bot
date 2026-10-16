package main

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
)

// Bootstrap has one worker outside the bulk-maintenance class; all work still
// acquires the existing shared SQL budget before any session or transaction.
func realityLane(parent *worker.WorkBudget, bulk *worker.WorkClass, initial bool) (string, int, func(context.Context, func(context.Context) error) error) {
	if initial {
		return "global-reality-bootstrap", 1, parent.Do
	}
	return "global-reality", 8, bulk.Do
}
