package workflows

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/definitions"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/workflow"
)

var (
	ErrInvalidRequest    = errors.New("invalid_request")
	ErrUnsupportedPolicy = errors.New("unsupported_policy")
	uuidPattern          = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

type StartRequest struct {
	DefinitionID string
	Input        workflow.Payload
}

type StartedWorkflow struct {
	ID        string
	Status    workflow.Status
	CreatedAt time.Time
}

type TaskStart struct {
	WorkflowID  string
	Key         string
	Input       workflow.Payload
	MaxAttempts int32
	CreatedAt   time.Time
}

type DefinitionRepository interface {
	FindByID(context.Context, string) (definitions.PublishedDefinition, error)
}

type Repository interface {
	Begin(context.Context) (Transaction, error)
}

// All writes belong to one READ COMMITTED transaction owned by the service.
type Transaction interface {
	InsertWorkflow(context.Context, StartRequest) (StartedWorkflow, error)
	InsertTask(context.Context, TaskStart) error
	Enqueue(context.Context, StartedWorkflow) error
	Commit() error
	Rollback() error
}

type Service struct {
	Definitions DefinitionRepository
	Repository  Repository
}

func (s Service) StartWorkflow(ctx context.Context, request StartRequest) (StartedWorkflow, error) {
	if !uuidPattern.MatchString(request.DefinitionID) {
		return StartedWorkflow{}, ErrInvalidRequest
	}
	request.Input.Value = slices.Clone(request.Input.Value)
	if len(request.Input.Value) == 0 {
		request.Input.Value = json.RawMessage("null")
	}
	if !utf8.Valid(request.Input.Value) || !json.Valid(request.Input.Value) {
		return StartedWorkflow{}, ErrInvalidRequest
	}
	// Published definitions cannot change, so this read needs no execution lock.
	published, err := s.Definitions.FindByID(ctx, request.DefinitionID)
	if err != nil {
		return StartedWorkflow{}, err
	}
	if err := published.Definition.ValidateSequence(); err != nil {
		return StartedWorkflow{}, err
	}
	tasks := published.Definition.Tasks()
	for _, task := range tasks {
		if task.MaxAttempts != 1 {
			return StartedWorkflow{}, ErrUnsupportedPolicy
		}
	}
	tx, err := s.Repository.Begin(ctx)
	if err != nil {
		return StartedWorkflow{}, fmt.Errorf("begin workflow: %w", err)
	}
	defer tx.Rollback()
	run, err := tx.InsertWorkflow(ctx, request)
	if err != nil {
		return StartedWorkflow{}, err
	}
	for _, task := range tasks {
		input := workflow.Payload{Value: json.RawMessage("null")}
		if len(task.DependsOn) == 0 {
			input = request.Input
		}
		if err := tx.InsertTask(ctx, TaskStart{WorkflowID: run.ID, Key: task.ID, Input: input,
			MaxAttempts: task.MaxAttempts, CreatedAt: run.CreatedAt}); err != nil {
			return StartedWorkflow{}, err
		}
	}
	if err := tx.Enqueue(ctx, run); err != nil {
		return StartedWorkflow{}, err
	}
	if err := tx.Commit(); err != nil {
		return StartedWorkflow{}, fmt.Errorf("commit workflow: %w", err)
	}
	return run, nil
}
