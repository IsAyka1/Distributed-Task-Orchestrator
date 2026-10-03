package definitions

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/workflow"
)

var (
	ErrDefinitionExists    = errors.New("definition_version_conflict")
	ErrDefinitionNotFound  = errors.New("definition_not_found")
	ErrInvalidDefinitionID = errors.New("invalid_definition_id")
)

type DefinitionKey struct {
	Provider string
	Name     string
	Version  int64
}

type PublishedDefinition struct {
	ID         string
	CreatedAt  time.Time
	Definition *workflow.Definition
}

// Insert must atomically append one definition or report a duplicate identity.
// It must never update an existing version, even when its content is identical.
type Repository interface {
	Insert(context.Context, *workflow.Definition) (PublishedDefinition, error)
	Find(context.Context, DefinitionKey) (PublishedDefinition, error)
}

type Service struct{ Repository Repository }

func (s Service) Publish(ctx context.Context, spec workflow.DefinitionSpec) (PublishedDefinition, error) {
	definition, err := ValidatePublication(spec)
	if err != nil {
		return PublishedDefinition{}, err
	}
	return s.Repository.Insert(ctx, definition)
}

func (s Service) Get(ctx context.Context, key DefinitionKey) (PublishedDefinition, error) {
	if !validIdentity(key.Provider) || !validIdentity(key.Name) || key.Version <= 0 {
		return PublishedDefinition{}, fmt.Errorf("%w: invalid identity", workflow.ErrInvalidDefinition)
	}
	return s.Repository.Find(ctx, key)
}

// Publication accepts DAGs; execution applies the narrower sequence restriction.
func ValidatePublication(spec workflow.DefinitionSpec) (*workflow.Definition, error) {
	if !validIdentity(spec.Provider) || !validIdentity(spec.Name) {
		return nil, fmt.Errorf("%w: invalid identity encoding", workflow.ErrInvalidDefinition)
	}
	definition, err := workflow.NewDefinition(spec)
	if err != nil {
		return nil, err
	}
	for _, task := range definition.Tasks() {
		if !validIdentity(task.ID) || task.Type != workflow.TaskTypeActivity {
			return nil, fmt.Errorf("%w: task id and activity type required", workflow.ErrInvalidDefinition)
		}
	}
	return definition, nil
}

// PostgreSQL text/JSONB cannot preserve invalid UTF-8 or the zero code point.
func validIdentity(value string) bool {
	return value != "" && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}
