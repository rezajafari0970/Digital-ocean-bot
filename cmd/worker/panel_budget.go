package main

import (
	"context"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"os"
	"strconv"
	"strings"
)

// A profile retains both its lease and the panel configuration fence. Proofs
// have a separate one-session reservation, including one retired-server lane.
// Keep four connections beyond all long-lived owners for nested SQL, discovery
// and heartbeat publication. Independent admission pools must not sum to a full
// database pool: their holders need additional SQL before they can release it.
func newPanelWorkBudgets(role worker.Role, proofConsumers int) (*worker.WorkBudget, *worker.WorkClass, func(context.Context, func(context.Context) error) error) {
	limit := (role.ConnectionBudget() - proofConsumers - 1 - 1 - 4) / 2
	if limit > 10 {
		limit = 10
	}
	if limit < 1 {
		panic("panel SQL reservations exceed connection budget")
	}
	parent := worker.NewWorkBudget(limit)
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
