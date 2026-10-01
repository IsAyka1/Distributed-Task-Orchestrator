package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/application"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/storage/postgres"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/workflow"
)

type definitionInput struct {
	Provider string      `json:"provider"`
	Name     string      `json:"name"`
	Version  int64       `json:"version"`
	Tasks    []taskInput `json:"tasks"`
}
type taskInput struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	DependsOn   []string `json:"depends_on"`
	MaxAttempts *int64   `json:"max_attempts,omitempty"`
}
type definitionOutput struct {
	ID         string          `json:"id"`
	CreatedAt  time.Time       `json:"created_at"`
	Definition definitionInput `json:"definition"`
}

func definitionCommand(ctx context.Context, db *sql.DB, action string) error {
	var input definitionInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		return err
	}
	service := application.Definitions{Store: postgres.DefinitionStore{DB: db}}
	var result application.PublishedDefinition
	var err error
	switch action {
	case "publish":
		spec := workflow.DefinitionSpec{Provider: input.Provider, Name: input.Name, Version: input.Version}
		for _, t := range input.Tasks {
			spec.Tasks = append(spec.Tasks, workflow.TaskSpec{ID: t.ID, Type: t.Type, DependsOn: t.DependsOn, MaxAttempts: t.MaxAttempts})
		}
		result, err = service.Publish(ctx, spec)
	case "get":
		result, err = service.Get(ctx, application.DefinitionKey{Provider: input.Provider, Name: input.Name, Version: input.Version})
	default:
		return fmt.Errorf("unknown definition action")
	}
	if err != nil {
		return err
	}
	definition := result.Definition
	output := definitionOutput{ID: result.ID, CreatedAt: result.CreatedAt, Definition: definitionInput{Provider: definition.Provider(), Name: definition.Name(), Version: definition.Version()}}
	for _, t := range definition.Tasks() {
		attempts := int64(t.MaxAttempts)
		dependencies := append([]string{}, t.DependsOn...)
		output.Definition.Tasks = append(output.Definition.Tasks, taskInput{ID: t.ID, Type: t.Type, DependsOn: dependencies, MaxAttempts: &attempts})
	}
	return json.NewEncoder(os.Stdout).Encode(output)
}
