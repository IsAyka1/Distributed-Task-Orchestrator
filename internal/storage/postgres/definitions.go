package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/application"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/workflow"
	"github.com/jackc/pgx/v5/pgconn"
)

type DefinitionStore struct{ DB *sql.DB }

var _ application.DefinitionStore = DefinitionStore{}

type definitionDocument struct {
	Tasks []definitionTask `json:"tasks"`
}
type definitionTask struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	DependsOn   []string `json:"depends_on"`
	MaxAttempts *int64   `json:"max_attempts,omitempty"`
}

func (s DefinitionStore) Insert(ctx context.Context, definition *workflow.Definition) (application.PublishedDefinition, error) {
	document := definitionDocument{Tasks: make([]definitionTask, 0, len(definition.Tasks()))}
	for _, task := range definition.Tasks() {
		attempts := int64(task.MaxAttempts)
		document.Tasks = append(document.Tasks, definitionTask{ID: task.ID, Type: task.Type, DependsOn: task.DependsOn, MaxAttempts: &attempts})
	}
	content, err := json.Marshal(document)
	if err != nil {
		return application.PublishedDefinition{}, fmt.Errorf("encode definition: %w", err)
	}
	published := application.PublishedDefinition{Definition: definition}
	// One statement is the complete publication transaction; uniqueness arbitrates races.
	err = s.DB.QueryRowContext(ctx, `INSERT INTO orchestrator.workflow_definitions
 (id, provider, name, version, definition) VALUES (gen_random_uuid(), $1, $2, $3, $4)
 RETURNING id::text, created_at`, definition.Provider(), definition.Name(), definition.Version(), content).Scan(&published.ID, &published.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "workflow_definitions_identity" {
			return application.PublishedDefinition{}, application.ErrDefinitionExists
		}
		return application.PublishedDefinition{}, fmt.Errorf("publish definition: %w", err)
	}
	return published, nil
}

func (s DefinitionStore) Find(ctx context.Context, key application.DefinitionKey) (application.PublishedDefinition, error) {
	var published application.PublishedDefinition
	var content []byte
	err := s.DB.QueryRowContext(ctx, `SELECT id::text, created_at, definition
 FROM orchestrator.workflow_definitions WHERE name = $1 AND version = $2 AND provider = $3`, key.Name, key.Version, key.Provider).Scan(&published.ID, &published.CreatedAt, &content)
	if errors.Is(err, sql.ErrNoRows) {
		return application.PublishedDefinition{}, application.ErrDefinitionNotFound
	}
	if err != nil {
		return application.PublishedDefinition{}, fmt.Errorf("read definition: %w", err)
	}
	var document definitionDocument
	if err := json.Unmarshal(content, &document); err != nil {
		return application.PublishedDefinition{}, fmt.Errorf("decode stored definition: %w", err)
	}
	spec := workflow.DefinitionSpec{Provider: key.Provider, Name: key.Name, Version: key.Version}
	for _, task := range document.Tasks {
		spec.Tasks = append(spec.Tasks, workflow.TaskSpec{ID: task.ID, Type: task.Type, DependsOn: task.DependsOn, MaxAttempts: task.MaxAttempts})
	}
	published.Definition, err = application.ValidatePublication(spec)
	if err != nil {
		return application.PublishedDefinition{}, fmt.Errorf("invalid stored definition: %w", err)
	}
	return published, nil
}
