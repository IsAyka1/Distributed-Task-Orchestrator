package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"time"

	taskRepository "github.com/IsAyka1/Distributed-Task-Orchestrator/internal/repositories/tasks"
	workflowRepository "github.com/IsAyka1/Distributed-Task-Orchestrator/internal/repositories/workflows"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/queue"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/tasks"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/workflows"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/storage/postgres"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/worker"
)

func runWorker(ctx context.Context, db *sql.DB, timeout time.Duration) error {
	registry, err := worker.NewRegistry(
		worker.Registration{Key: worker.DemoA, Activity: worker.DemoActivity{}},
		worker.Registration{Key: worker.DemoB, Activity: worker.DemoActivity{}},
		worker.Registration{Key: worker.DemoC, Activity: worker.DemoActivity{}},
	)
	if err != nil {
		return err
	}
	repository := workflowRepository.Repository{DB: db}
	runner := worker.Runner{
		Queue:     queue.Service{Repository: postgres.ClaimRepository{DB: db}},
		Completer: tasks.Service{Repository: taskRepository.Repository{DB: db}},
		Evaluator: workflows.Service{Repository: repository}, Repository: repository,
		Registry: registry, Clock: worker.SystemClock{},
		Config: worker.Config{WorkerID: "worker-" + rand.Text(), PollInterval: 100 * time.Millisecond, OperationTimeout: timeout, LeaseDuration: 2 * time.Minute},
	}
	if err := runner.Run(ctx); err != nil {
		return errors.New("worker stopped; check database availability and persisted workflow state")
	}
	return nil
}
