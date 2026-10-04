package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/storage/postgres"
)

type clockSnapshot struct {
	Application time.Time `json:"application"`
	Database    time.Time `json:"database"`
}

func main() {
	if err := run(os.Args[1:], postgres.Open); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, open func(context.Context, string) (*sql.DB, error)) error {
	flags := flag.NewFlagSet("probe", flag.ContinueOnError)
	dir := flags.String("migrations", "", "test migration directory")
	definitionAction := flags.String("definition", "", "test definition action")
	workflowAction := flags.String("workflow", "", "test workflow action")
	claim := flags.Bool("claim", false, "test task claim")
	instant := flags.String("now", "", "fixed application time")
	if err := flags.Parse(args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := open(ctx, os.Getenv("DATABASE_URL"))
	if err == nil {
		defer conn.Close()
		if *dir != "" {
			err = postgres.Migrate(ctx, conn, os.DirFS(*dir))
		} else if *definitionAction != "" {
			err = definitionCommand(ctx, conn, *definitionAction)
		} else if *claim {
			err = claimCommand(ctx, conn)
		} else if *workflowAction != "" {
			err = workflowCommand(ctx, conn, *workflowAction)
		} else {
			var app, db time.Time
			app, err = time.Parse(time.RFC3339Nano, *instant)
			if err == nil {
				db, err = postgres.Now(ctx, conn)
			}
			if err == nil {
				err = json.NewEncoder(os.Stdout).Encode(clockSnapshot{Application: app, Database: db})
			}
		}
	}
	return err
}
