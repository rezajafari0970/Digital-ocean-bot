// Create-only bootstrap for a new installation; never reset an existing user.
package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	_ "github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/migrate"
	"golang.org/x/crypto/bcrypt"
	"log"
	"os"
	"strings"
	"time"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal("database configuration failed")
	}
	defer db.Close()
	if err = (migrate.Runner{DB: db, Dir: "migrations"}).Up(ctx); err != nil {
		log.Fatal(err)
	}
	// No permission marker means migration-only (normal update and existing installations).
	if os.Getenv("DOB_FRESH_INSTALL") != "1" {
		fmt.Println("Migrations ready; existing users preserved")
		return
	}
	raw, err := os.ReadFile("/etc/digital-ocean-bot/setup-token")
	if err != nil {
		log.Fatal("setup token unavailable")
	}
	grant := strings.TrimSpace(string(raw))
	if len(grant) != 43 {
		log.Fatal("invalid setup token")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "LOCK TABLE admin_users IN EXCLUSIVE MODE"); err != nil {
		log.Fatal(err)
	}
	var count int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM admin_users").Scan(&count); err != nil {
		log.Fatal(err)
	}
	if count == 0 {
		h := sha256.Sum256([]byte(grant))
		_, err = tx.ExecContext(ctx, `INSERT INTO admin_users(id,username,password_hash,role,enabled,must_change_password,bootstrap_token_hash) VALUES(gen_random_uuid(),'admin',$1,'admin',true,true,$2)`, string(hash), h[:])
		if err != nil {
			log.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Initial administrator checked; existing credentials preserved")
}
