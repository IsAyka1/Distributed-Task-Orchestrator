package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	definitions "github.com/IsAyka1/Distributed-Task-Orchestrator/internal/repositories/definitions"
	repository "github.com/IsAyka1/Distributed-Task-Orchestrator/internal/repositories/workflows"
	service "github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/workflows"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/workflow"
)

type workflowInput struct {
	DefinitionID string          `json:"definition_id"`
	WorkflowID   string          `json:"workflow_id"`
	Input        json.RawMessage `json:"input"`
}

type workflowOutput struct {
	ID        string          `json:"id"`
	Status    workflow.Status `json:"status"`
	CreatedAt time.Time       `json:"created_at"`
}

func workflowCommand(ctx context.Context, db *sql.DB, action string) error {
	if action != "start" && action != "insert" && action != "evaluate" && action != "evaluate-one-connection" {
		return fmt.Errorf("unknown workflow action")
	}
	var input workflowInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		return err
	}
	if action == "insert" {
		return insertWorkflowRecords(ctx, db, input)
	}
	s := service.Service{Definitions: definitions.Repository{DB: db}, Repository: repository.Repository{DB: db}}
	if action == "evaluate-one-connection" {
		db.SetMaxOpenConns(1)
	}
	if action == "evaluate" || action == "evaluate-one-connection" {
		return s.EvaluateWorkflow(ctx, input.WorkflowID)
	}
	run, err := s.StartWorkflow(ctx, service.StartRequest{DefinitionID: input.DefinitionID, Input: workflow.Payload{Value: input.Input}})
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(workflowOutput{ID: run.ID, Status: run.Status, CreatedAt: run.CreatedAt})
}

type insertedWorkflowRecords struct {
	Workflow service.StartedWorkflow `json:"workflow"`
	Task     service.StartedTask     `json:"task"`
	Wakeup   service.Wakeup          `json:"wakeup"`
}

func insertWorkflowRecords(ctx context.Context, db *sql.DB, input workflowInput) error {
	tx, err := (repository.Repository{DB: db}).Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var records insertedWorkflowRecords
	records.Workflow, err = tx.InsertWorkflow(ctx, service.StartRequest{
		DefinitionID: input.DefinitionID, Input: workflow.Payload{Value: input.Input}})
	if err != nil {
		return err
	}
	records.Task, err = tx.InsertTask(ctx, service.TaskStart{WorkflowID: records.Workflow.ID,
		Key: "A", Input: workflow.Payload{Value: input.Input}, MaxAttempts: 1, CreatedAt: records.Workflow.CreatedAt})
	if err != nil {
		return err
	}
	records.Wakeup, err = tx.Enqueue(ctx, records.Workflow)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(records)
}
