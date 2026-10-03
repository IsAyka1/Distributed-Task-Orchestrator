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
	Input        json.RawMessage `json:"input"`
}

type workflowOutput struct {
	ID        string          `json:"id"`
	Status    workflow.Status `json:"status"`
	CreatedAt time.Time       `json:"created_at"`
}

func workflowCommand(ctx context.Context, db *sql.DB, action string) error {
	if action != "start" {
		return fmt.Errorf("unknown workflow action")
	}
	var input workflowInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		return err
	}
	s := service.Service{Definitions: definitions.Repository{DB: db}, Repository: repository.Repository{DB: db}}
	run, err := s.StartWorkflow(ctx, service.StartRequest{DefinitionID: input.DefinitionID, Input: workflow.Payload{Value: input.Input}})
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(workflowOutput{ID: run.ID, Status: run.Status, CreatedAt: run.CreatedAt})
}
