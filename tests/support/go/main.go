package main

import (
	"context"
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
	dir := flag.String("migrations", "", "test migration directory")
	definitionAction := flag.String("definition", "", "test definition action")
	instant := flag.String("now", "", "fixed application time")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := postgres.Open(ctx, os.Getenv("DATABASE_URL"))
	if err == nil {
		defer conn.Close()
		if *dir != "" {
			err = postgres.Migrate(ctx, conn, os.DirFS(*dir))
		} else if *definitionAction != "" {
			err = definitionCommand(ctx, conn, *definitionAction)
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
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
