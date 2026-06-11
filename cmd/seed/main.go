package main

import (
	"context"
	"fmt"
	"os"

	"CleanCaregent/internal/config"
	"CleanCaregent/internal/migrate"
	mysqlclient "CleanCaregent/internal/platform/mysql"
	"CleanCaregent/internal/seed"
)

func main() {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}
	if !cfg.MySQL.Enabled {
		fmt.Fprintln(os.Stderr, "mysql must be enabled")
		os.Exit(1)
	}
	ctx := context.Background()
	db, err := mysqlclient.Open(ctx, cfg.MySQL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open mysql: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()
	if err := migrate.Up(ctx, db); err != nil {
		fmt.Fprintf(os.Stderr, "migrate mysql: %v\n", err)
		os.Exit(1)
	}
	if err := seed.MockBusinessData(ctx, db); err != nil {
		fmt.Fprintf(os.Stderr, "seed mock data: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("mock business data seeded")
}
