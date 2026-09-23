package main

import (
	"bufio"
	"context"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"log"
	"os"
	"strings"
)

func main() {
	username := strings.TrimSpace(os.Getenv("ADMIN_USERNAME"))
	if username == "" {
		log.Fatal("ADMIN_USERNAME required")
	}
	reader := bufio.NewReader(os.Stdin)
	password, err := reader.ReadString('\n')
	if err != nil {
		log.Fatal("password must be supplied on stdin")
	}
	password = strings.TrimSpace(password)
	hash, err := auth.HashPassword(password)
	if err != nil {
		log.Fatal(err)
	}
	password = ""
	ctx := context.Background()
	a, err := app.Bootstrap(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer a.Close()
	var id string
	err = a.DB.QueryRowContext(ctx, `INSERT INTO admin_users(id,username,password_hash,role,enabled) VALUES(gen_random_uuid(),$1,$2,'admin',true) ON CONFLICT(username) DO UPDATE SET password_hash=EXCLUDED.password_hash,role='admin',enabled=true,updated_at=now() RETURNING id::text`, username, hash).Scan(&id)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("admin ready: %s (%s)\n", username, id)
}
