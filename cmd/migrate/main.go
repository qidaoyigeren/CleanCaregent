package main

import (
	"context"
	"fmt"
	"os"

	"CleanCaregent/internal/config"
	"CleanCaregent/internal/migrate"
	mysqlclient "CleanCaregent/internal/platform/mysql"
)

func main() {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}
	if !cfg.MySQL.Enabled {
		fmt.Fprintln(os.Stderr, "mysql.enabled must be true")
		os.Exit(1)
	}

	db, err := mysqlclient.Open(context.Background(), cfg.MySQL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect mysql: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := migrate.Up(context.Background(), db); err != nil {
		fmt.Fprintf(os.Stderr, "run migrations: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("migrations applied")
}
