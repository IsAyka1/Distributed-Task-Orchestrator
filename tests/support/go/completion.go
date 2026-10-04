package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"

	repository "github.com/IsAyka1/Distributed-Task-Orchestrator/internal/repositories/tasks"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/tasks"
)

func completionCommand(ctx context.Context, db *sql.DB) error {
	var input tasks.CompletionRequest
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	return (tasks.Service{Repository: repository.Repository{DB: db}}).CompleteAttempt(ctx, input)
}
