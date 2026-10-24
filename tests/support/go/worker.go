package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"time"

	taskRepository "github.com/IsAyka1/Distributed-Task-Orchestrator/internal/repositories/tasks"
	workflowRepository "github.com/IsAyka1/Distributed-Task-Orchestrator/internal/repositories/workflows"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/queue"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/tasks"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/workflows"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/storage/postgres"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/worker"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/workflow"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/tests/support/clock"
)

type workerRequest struct {
	Steps  int
	Now    time.Time
	FailAt string
}
type activityCall struct {
	Key, TaskID string
	Input       workflow.Payload
}
type workerResult struct {
	Calls  []activityCall
	Worked []bool
}
type observedActivity struct {
	db          *sql.DB
	key, failAt string
	calls       *[]activityCall
}

func (a observedActivity) Execute(ctx context.Context, input workflow.Payload, key string) (workflow.Payload, error) {
	if a.db.Stats().InUse != 0 {
		return workflow.Payload{}, errors.New("activity called with a connection held")
	}
	var taskStatus, attemptStatus string
	if err := a.db.QueryRowContext(ctx, workerLocksSQL, key).Scan(&taskStatus, &attemptStatus); err != nil {
		return workflow.Payload{}, err
	}
	if taskStatus != "RUNNING" || attemptStatus != "RUNNING" {
		return workflow.Payload{}, errors.New("claim is not committed")
	}
	*a.calls = append(*a.calls, activityCall{Key: a.key, TaskID: key, Input: input})
	if a.key == a.failAt {
		return workflow.Payload{}, errors.New("private activity failure")
	}
	return (worker.DemoActivity{}).Execute(ctx, input, key)
}

func workerCommand(ctx context.Context, db *sql.DB) error {
	var request workerRequest
	if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
		return err
	}
	if request.Steps < 1 || request.Steps > 20 || request.Now.IsZero() {
		return errors.New("invalid worker probe")
	}
	db.SetMaxOpenConns(1)
	result := workerResult{Calls: []activityCall{}}
	entries := make([]worker.Registration, 0, 3)
	for _, key := range []string{worker.DemoA, worker.DemoB, worker.DemoC} {
		entries = append(entries, worker.Registration{Key: key, Activity: observedActivity{db: db, key: key, failAt: request.FailAt, calls: &result.Calls}})
	}
	registry, err := worker.NewRegistry(entries...)
	if err != nil {
		return err
	}
	repository := workflowRepository.Repository{DB: db}
	runner := worker.Runner{Queue: queue.Service{Repository: postgres.ClaimRepository{DB: db}},
		Completer: tasks.Service{Repository: taskRepository.Repository{DB: db}}, Evaluator: workflows.Service{Repository: repository},
		Repository: repository, Registry: registry, Clock: clock.New(request.Now),
		Config: worker.Config{WorkerID: "probe-worker", PollInterval: time.Second, OperationTimeout: time.Second, LeaseDuration: 10 * time.Second}}
	for i := 0; i < request.Steps; i++ {
		worked, err := runner.Step(ctx)
		if err != nil {
			return err
		}
		result.Worked = append(result.Worked, worked)
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
