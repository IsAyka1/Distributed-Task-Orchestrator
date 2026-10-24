package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"

	httpapi "github.com/IsAyka1/Distributed-Task-Orchestrator/internal/api/http"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/config"
	definitionRepository "github.com/IsAyka1/Distributed-Task-Orchestrator/internal/repositories/definitions"
	workflowRepository "github.com/IsAyka1/Distributed-Task-Orchestrator/internal/repositories/workflows"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/definitions"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/workflows"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/worker"
)

func runApplication(parent context.Context, db *sql.DB, cfg config.Config) error {
	listener, err := net.Listen("tcp", cfg.HTTPAddress)
	if err != nil {
		return errors.New("HTTP listener failed; check HTTP_ADDR and port availability")
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	repository := definitionRepository.Repository{DB: db}
	handler := httpapi.Handler{Inspection: workflows.Inspector{Repository: workflowRepository.Repository{DB: db}},
		Definitions: definitions.Service{Repository: repository},
		Workflows:   workflows.Service{Definitions: repository, Repository: workflowRepository.Repository{DB: db}},
		Clock:       worker.SystemClock{}, Timeout: cfg.Timeout,
	}
	results := make(chan error, 2)
	go func() { results <- httpapi.Serve(ctx, listener, handler, cfg.Timeout) }()
	go func() { results <- runWorker(ctx, db, cfg.Timeout) }()
	fmt.Printf("HTTP listening on %s\n", listener.Addr())
	first := <-results
	cancel()
	second := <-results
	if first != nil || second != nil {
		return errors.New("application stopped; check HTTP and database availability and persisted workflow state")
	}
	return nil
}
