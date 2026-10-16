package main

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
)

func newPanelWorkBudgets() (*worker.WorkBudget, *worker.WorkClass, func(context.Context, func(context.Context) error) error) {
	parent := worker.NewWorkBudget(10)
	bulk := worker.NewWorkClass(parent, 2)
	return parent, bulk, worker.NewWorkClass(parent, 2).Do
}
