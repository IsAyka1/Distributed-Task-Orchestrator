// Package workflow contains pure workflow definitions and validation.
package workflow

import (
	"errors"
	"fmt"
	"math"
	"slices"
)

var (
	ErrInvalidDefinition   = errors.New("invalid_definition")
	ErrUnsupportedWorkflow = errors.New("unsupported_workflow")
)

// DefinitionSpec is mutable constructor input, not a published definition.
// Publication identity, timestamps and uniqueness are owned by storage/application.
type DefinitionSpec struct {
	Provider string
	Name     string
	Version  int64
	Tasks    []TaskSpec
}

// TaskSpec distinguishes an omitted limit (nil) from an explicit invalid zero.
// Boundary adapters must reject fractional and out-of-range JSON numbers.
type TaskSpec struct {
	ID          string
	Type        string
	DependsOn   []string
	MaxAttempts *int64
}

// TaskDefinition is a detached snapshot with a normalized, finite attempt limit.
type TaskDefinition struct {
	ID          string
	Type        string
	DependsOn   []string
	MaxAttempts int32
}

// Definition owns its data. Construct with NewDefinition; the zero value is invalid.
// Copies may share private immutable storage. All returned slices are detached.
type Definition struct {
	provider string
	name     string
	version  int64
	tasks    []TaskDefinition
}

// NewDefinition validates a DAG and snapshots input without retaining caller data.
// Validation permits branching; execution callers must also use ValidateSequence
// until stage 5. This function performs no I/O and never modifies input.
func NewDefinition(spec DefinitionSpec) (*Definition, error) {
	if spec.Provider == "" || spec.Name == "" || spec.Version <= 0 || len(spec.Tasks) == 0 {
		return nil, fmt.Errorf("%w: provider, name, positive version and tasks are required", ErrInvalidDefinition)
	}
	definition := &Definition{provider: spec.Provider, name: spec.Name, version: spec.Version}
	definition.tasks = make([]TaskDefinition, len(spec.Tasks))
	indices := make(map[string]int, len(spec.Tasks))
	for i, task := range spec.Tasks {
		if task.ID == "" || task.Type != "activity" {
			return nil, fmt.Errorf("%w: task %d requires an ID and activity type", ErrInvalidDefinition, i)
		}
		if _, exists := indices[task.ID]; exists {
			return nil, fmt.Errorf("%w: duplicate task ID at task %d", ErrInvalidDefinition, i)
		}
		limit := int64(1)
		if task.MaxAttempts != nil {
			limit = *task.MaxAttempts
		}
		if limit < 1 || limit > math.MaxInt32 {
			return nil, fmt.Errorf("%w: task %d has invalid max_attempts", ErrInvalidDefinition, i)
		}
		indices[task.ID] = i
		definition.tasks[i] = TaskDefinition{ID: task.ID, Type: task.Type, DependsOn: slices.Clone(task.DependsOn), MaxAttempts: int32(limit)}
	}
	// Kahn's algorithm visits every component without recursive stack growth.
	remaining := make([]int, len(definition.tasks))
	successors := make([][]int, len(definition.tasks))
	ready := make([]int, 0, len(definition.tasks))
	for i, task := range definition.tasks {
		seen := make(map[string]bool, len(task.DependsOn))
		for _, dependency := range task.DependsOn {
			predecessor, exists := indices[dependency]
			if !exists || predecessor == i || seen[dependency] {
				return nil, fmt.Errorf("%w: task %d has an unknown, self or duplicate dependency", ErrInvalidDefinition, i)
			}
			seen[dependency] = true
			successors[predecessor] = append(successors[predecessor], i)
		}
		remaining[i] = len(task.DependsOn)
		if remaining[i] == 0 {
			ready = append(ready, i)
		}
	}
	for head := 0; head < len(ready); head++ {
		for _, successor := range successors[ready[head]] {
			remaining[successor]--
			if remaining[successor] == 0 {
				ready = append(ready, successor)
			}
		}
	}
	if len(ready) != len(definition.tasks) {
		return nil, fmt.Errorf("%w: dependency cycle", ErrInvalidDefinition)
	}
	return definition, nil
}

func (d Definition) Provider() string { return d.provider }
func (d Definition) Name() string     { return d.name }
func (d Definition) Version() int64   { return d.version }

// Tasks returns snapshots in declaration order, not execution order.
func (d Definition) Tasks() []TaskDefinition {
	tasks := slices.Clone(d.tasks)
	for i := range tasks {
		tasks[i].DependsOn = slices.Clone(tasks[i].DependsOn)
	}
	return tasks
}

// ValidateSequence enforces the pre-stage-5 graph gate. Attempt-policy gating is
// separate: publication accepts larger finite limits before execution supports them.
func (d Definition) ValidateSequence() error {
	if len(d.tasks) == 0 {
		return ErrInvalidDefinition
	}
	roots := 0
	successors := make(map[string]int, len(d.tasks))
	for _, task := range d.tasks {
		switch len(task.DependsOn) {
		case 0:
			roots++
		case 1:
			predecessor := task.DependsOn[0]
			successors[predecessor]++
			if successors[predecessor] > 1 {
				return ErrUnsupportedWorkflow
			}
		default:
			return ErrUnsupportedWorkflow
		}
	}
	// A validated finite DAG with one root, no joins and no forks is one chain.
	if roots != 1 {
		return ErrUnsupportedWorkflow
	}
	return nil
}
