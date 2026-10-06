// Command upcloud-trial-enable applies an explicitly authorized trial-mode
// transition using the same transaction as the authenticated admin API.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	_ "github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/capacity"
	"os"
	"time"
)

func main() {
	account := flag.String("account", "", "UpCloud account UUID")
	version := flag.Int64("block-version", 0, "exact current TRIAL_FIREWALL version")
	accepted := flag.Bool("accept-restricted-egress", false, "authorize restricted trial deployment")
	flag.Parse()
	if !*accepted || *account == "" || *version <= 0 {
		fmt.Fprintln(os.Stderr, "account, exact block version and restricted-egress acceptance required")
		os.Exit(2)
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL required")
		os.Exit(2)
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "database unavailable")
		os.Exit(1)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err = capacity.EnableUpCloudTrial(ctx, db, *account, *version, "authorized-local-operator"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("trial-compatible mode enabled; provider restrictions remain")
}
