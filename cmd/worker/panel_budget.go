package main

import (
	"context"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"os"
	"strconv"
	"strings"
)

func newPanelWorkBudgets() (*worker.WorkBudget, *worker.WorkClass, func(context.Context, func(context.Context) error) error) {
	parent := worker.NewWorkBudget(10)
	bulk := worker.NewWorkClass(parent, 2)
	return parent, bulk, worker.NewWorkClass(parent, 2).Do
}

// Proofs have independent serving/retirement reservations. High capacity is
// qualified only for the split panels role; combined-role work remains capped.
func newProofWorkBudgets(role worker.Role, configured string) (*worker.WorkBudget, *worker.WorkBudget, int) {
	limit := 2
	if strings.TrimSpace(configured) != "" {
		n, err := strconv.Atoi(strings.TrimSpace(configured))
		if err != nil || n < 1 || n > 18 {
			panic(fmt.Sprintf("invalid DOB_RESIDENTIAL_PROOF_CONCURRENCY %q", configured))
		}
		limit = n
	}
	if role != worker.RolePanels && limit > 2 {
		limit = 2
	}
	return worker.NewWorkBudget(limit), worker.NewWorkBudget(1), limit
}
func configuredProofWorkBudgets(role worker.Role) (*worker.WorkBudget, *worker.WorkBudget, int) {
	return newProofWorkBudgets(role, os.Getenv("DOB_RESIDENTIAL_PROOF_CONCURRENCY"))
}
