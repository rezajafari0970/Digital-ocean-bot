package main

import "github.com/rezajafari0970/Digital-ocean-bot/internal/worker"

// Bulk management, provider reads and identity renewal have independent bounds.
// Reserve two positions for each observation class even while bulk work runs.
// All five top-level operations still share the control role SQL pool of 24.
func newControlWorkBudgets() (*worker.WorkBudget, *worker.WorkClass, *worker.WorkClass, *worker.WorkClass) {
	parent := worker.NewWorkBudget(5)
	return parent, worker.NewWorkClass(parent, 1), worker.NewWorkClass(parent, 2), worker.NewWorkClass(parent, 2)
}
