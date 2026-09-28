package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/config"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/storage/postgres"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/migrations"
)

func run(ctx context.Context, args []string) error {
	if len(args) != 1 || (args[0] != "db-check" && args[0] != "migrate") {
		return fmt.Errorf("usage: orchestrator {db-check|migrate}")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	conn, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer conn.Close()
	if args[0] == "migrate" {
		if err := postgres.Migrate(ctx, conn, migrations.Files); err != nil {
			return fmt.Errorf("migration failed; check migrations, database access and DATABASE_TIMEOUT")
		}
	}
	return nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
