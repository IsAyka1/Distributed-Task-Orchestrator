package workflow

import (
	"errors"
	"fmt"
	"math"
)

var (
	ErrInvalidDefinition   = errors.New("invalid_definition")
	ErrUnsupportedWorkflow = errors.New("unsupported_workflow")
)

func validateDefinition(spec DefinitionSpec) error {
	if spec.Provider == "" || spec.Name == "" || spec.Version <= 0 || len(spec.Tasks) == 0 {
		return fmt.Errorf("%w: provider, name, positive version and tasks are required", ErrInvalidDefinition)
	}
	indices := make(map[string]int, len(spec.Tasks))
	for i, task := range spec.Tasks {
		if _, exists := indices[task.ID]; exists {
			return fmt.Errorf("%w: duplicate task ID at task %d", ErrInvalidDefinition, i)
		}
		if task.MaxAttempts != nil && (*task.MaxAttempts < 1 || *task.MaxAttempts > math.MaxInt32) {
			return fmt.Errorf("%w: task %d has invalid max_attempts", ErrInvalidDefinition, i)
		}
		indices[task.ID] = i
	}
	remaining := make([]int, len(spec.Tasks))
	successors := make([][]int, len(spec.Tasks))
	for i, task := range spec.Tasks {
		seen := make(map[string]bool, len(task.DependsOn))
		for _, dependency := range task.DependsOn {
			predecessor, exists := indices[dependency]
			if !exists || predecessor == i || seen[dependency] {
				return fmt.Errorf("%w: task %d has an unknown, self or duplicate dependency", ErrInvalidDefinition, i)
			}
			seen[dependency] = true
			successors[predecessor] = append(successors[predecessor], i)
		}
		remaining[i] = len(task.DependsOn)
	}
	return validateAcyclic(remaining, successors)
}

func validateAcyclic(remaining []int, successors [][]int) error {
	ready := make([]int, 0, len(remaining))
	for i, count := range remaining {
		if count == 0 {
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
	if len(ready) != len(remaining) {
		return fmt.Errorf("%w: dependency cycle", ErrInvalidDefinition)
	}
	return nil
}

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
	if roots != 1 {
		return ErrUnsupportedWorkflow
	}
	return nil
}
