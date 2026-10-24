package main

import "github.com/rezajafari0970/Digital-ocean-bot/internal/worker"

// At most two expensive owners and five total owners run together.
// Weighted admission bounds held SQL at 17, plus the role owner and five
// completion queries: 23 of 24 connections. Observation reserves still permit
// bulk6 + two provider2 + two identity3 concurrently, as before.
func newControlWorkBudgets() (*worker.WorkBudget, *worker.WorkClass, *worker.WorkClass, *worker.WorkClass, *worker.WorkClass) {
	parent := worker.NewWeightedWorkBudget(5, 17)
	expensive := worker.NewWorkClass(parent.WithWeight(6), 2)
	bulk := worker.NewWorkClass(expensive, 1)
	recovery := worker.NewWorkClass(expensive, 2)
	return parent, bulk, worker.NewWorkClass(parent.WithWeight(2), 2), worker.NewWorkClass(parent.WithWeight(3), 2), recovery
}

// Scheduler and observations do not queue behind the expensive class.
func newSchedulerWorkBudget(parent *worker.WorkBudget) *worker.WorkClass {
	return worker.NewWorkClass(parent.WithWeight(3), 1)
}
