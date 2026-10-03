package definitions

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	service "github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/definitions"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/workflow"
	"github.com/jackc/pgx/v5/pgconn"
)

type Repository struct{ DB *sql.DB }

var _ service.Repository = Repository{}

type definitionDocument struct {
	Tasks []definitionTask `json:"tasks"`
}
type definitionTask struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	DependsOn   []string `json:"depends_on"`
	MaxAttempts *int64   `json:"max_attempts,omitempty"`
}

func (s Repository) Insert(ctx context.Context, definition *workflow.Definition) (service.PublishedDefinition, error) {
	document := definitionDocument{Tasks: make([]definitionTask, 0, len(definition.Tasks()))}
	for _, task := range definition.Tasks() {
		attempts := int64(task.MaxAttempts)
		document.Tasks = append(document.Tasks, definitionTask{ID: task.ID, Type: task.Type, DependsOn: task.DependsOn, MaxAttempts: &attempts})
	}
	content, err := json.Marshal(document)
	if err != nil {
		return service.PublishedDefinition{}, fmt.Errorf("encode definition: %w", err)
	}
	published := service.PublishedDefinition{Definition: definition}
	// One statement is the complete publication transaction; uniqueness arbitrates races.
	err = s.DB.QueryRowContext(ctx, insertSQL, definition.Provider(), definition.Name(), definition.Version(), content).Scan(&published.ID, &published.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "workflow_definitions_identity" {
			return service.PublishedDefinition{}, service.ErrDefinitionExists
		}
		return service.PublishedDefinition{}, fmt.Errorf("publish definition: %w", err)
	}
	return published, nil
}

func (s Repository) Find(ctx context.Context, key service.DefinitionKey) (service.PublishedDefinition, error) {
	var published service.PublishedDefinition
	var content []byte
	err := s.DB.QueryRowContext(ctx, findSQL, key.Name, key.Version, key.Provider).Scan(&published.ID, &published.CreatedAt, &content)
	if errors.Is(err, sql.ErrNoRows) {
		return service.PublishedDefinition{}, service.ErrDefinitionNotFound
	}
	if err != nil {
		return service.PublishedDefinition{}, fmt.Errorf("read definition: %w", err)
	}
	return decodeDefinition(published, key, content)
}

func (s Repository) FindByID(ctx context.Context, id string) (service.PublishedDefinition, error) {
	var published service.PublishedDefinition
	var key service.DefinitionKey
	var content []byte
	err := s.DB.QueryRowContext(ctx, findByIDSQL, id).Scan(&published.ID, &published.CreatedAt,
		&key.Provider, &key.Name, &key.Version, &content)
	if errors.Is(err, sql.ErrNoRows) {
		return service.PublishedDefinition{}, service.ErrDefinitionNotFound
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && (pgErr.Code == "22P02" || pgErr.Code == "22021") {
			return service.PublishedDefinition{}, service.ErrInvalidDefinitionID
		}
		return service.PublishedDefinition{}, fmt.Errorf("read definition by id: %w", err)
	}
	return decodeDefinition(published, key, content)
}

func decodeDefinition(published service.PublishedDefinition, key service.DefinitionKey, content []byte) (service.PublishedDefinition, error) {
	var document definitionDocument
	if err := json.Unmarshal(content, &document); err != nil {
		return service.PublishedDefinition{}, fmt.Errorf("decode stored definition: %w", err)
	}
	spec := workflow.DefinitionSpec{Provider: key.Provider, Name: key.Name, Version: key.Version}
	for _, task := range document.Tasks {
		spec.Tasks = append(spec.Tasks, workflow.TaskSpec{ID: task.ID, Type: task.Type, DependsOn: task.DependsOn, MaxAttempts: task.MaxAttempts})
	}
	definition, err := service.ValidatePublication(spec)
	if err != nil {
		return service.PublishedDefinition{}, fmt.Errorf("invalid stored definition: %w", err)
	}
	published.Definition = definition
	return published, nil
}
