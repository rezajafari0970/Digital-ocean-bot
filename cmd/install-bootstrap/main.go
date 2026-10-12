// Create-only bootstrap for a new installation; never reset an existing user.
package main

import (
	"context"
	"database/sql"
	"fmt"
	_ "github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/migrate"
	"golang.org/x/crypto/bcrypt"
	"log"
	"os"
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
	if err = (migrate.Runner{DB: db, Dir: "migrations", PreserveData: os.Getenv("DOB_FRESH_INSTALL") != "1"}).Up(ctx); err != nil {
		log.Fatal(err)
	}
	// No permission marker means migration-only (normal update and existing installations).
	if os.Getenv("DOB_FRESH_INSTALL") != "1" {
		fmt.Println("Migrations ready; existing users preserved")
		return
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
		_, err = tx.ExecContext(ctx, `INSERT INTO admin_users(id,username,password_hash,role,enabled,must_change_password,bootstrap_token_hash) VALUES(gen_random_uuid(),'admin',$1,'admin',true,false,NULL)`, string(hash))
		if err != nil {
			log.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Initial administrator checked; existing credentials preserved")
}
