package main

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	httpapi "github.com/IsAyka1/Distributed-Task-Orchestrator/internal/api/http"
	definitionRepository "github.com/IsAyka1/Distributed-Task-Orchestrator/internal/repositories/definitions"
	workflowRepository "github.com/IsAyka1/Distributed-Task-Orchestrator/internal/repositories/workflows"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/definitions"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/workflows"
	testclock "github.com/IsAyka1/Distributed-Task-Orchestrator/tests/support/clock"
)

func httpCommand(ctx context.Context, db *sql.DB) error {
	instant, err := time.Parse(time.RFC3339, os.Getenv("TEST_HTTP_NOW"))
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	clock := testclock.New(instant)
	var starts workflows.Repository = workflowRepository.Repository{DB: db}
	if mode := os.Getenv("TEST_HTTP_CANCEL_START"); mode != "" {
		db.SetMaxOpenConns(1)
		starts = cancellingStartRepository{Repository: starts, db: db, clock: clock, mode: mode}
	}
	repository := definitionRepository.Repository{DB: db}
	handler := httpapi.Handler{Definitions: definitions.Service{Repository: repository},
		Workflows: workflows.Service{Definitions: repository, Repository: starts},
		Clock:     clock, Timeout: time.Second}
	fmt.Printf("HTTP listening on %s\n", listener.Addr())
	return httpapi.Serve(ctx, listener, handler, time.Second)
}
