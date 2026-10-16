package main

import "github.com/rezajafari0970/Digital-ocean-bot/internal/worker"

// Every non-observation control shares one aggregate class. This reserves two
// parent positions for provider discovery/reads while keeping total work at 3.
func newControlWorkBudgets() (*worker.WorkBudget, *worker.WorkClass, *worker.WorkClass) {
	parent := worker.NewWorkBudget(3)
	return parent, worker.NewWorkClass(parent, 1), worker.NewWorkClass(parent, 2)
}
