package engine

import (
	"errors"

	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/task"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/workflow"
)

var ErrInvalidState = errors.New("invalid_state")

type TaskSnapshot struct {
	Key   string
	State task.State
}

type Changes struct {
	Status workflow.Status
	Tasks  []TaskSnapshot
}

// Evaluate proposes changes from one consistent snapshot. Callers commit them atomically.
func Evaluate(definition workflow.Definition, status workflow.Status, tasks []TaskSnapshot) (Changes, error) {
	unchanged := Changes{Status: status}
	switch status {
	case workflow.Succeeded, workflow.Failed:
		// Redundant wakeups for terminal runs need no task inspection.
		return unchanged, nil
	case workflow.Pending, workflow.Running:
	default:
		return unchanged, ErrInvalidState
	}
	if err := definition.ValidateSequence(); err != nil {
		return unchanged, err
	}
	definitions := definition.Tasks()
	for _, d := range definitions {
		if d.MaxAttempts != 1 {
			return unchanged, task.ErrUnsupportedPolicy
		}
	}
	ordered, err := orderedSnapshots(definitions, tasks)
	if err != nil {
		return unchanged, err
	}
	frontier := len(ordered)
	for i, snapshot := range ordered {
		if status == workflow.Pending && snapshot.State.Status != task.Pending {
			return unchanged, ErrInvalidState
		}
		if frontier < len(ordered) && snapshot.State.Status != task.Pending {
			return unchanged, ErrInvalidState
		}
		if frontier == len(ordered) && snapshot.State.Status != task.Succeeded {
			frontier = i
		}
	}
	changes := unchanged
	if status == workflow.Pending {
		changes.Status, err = workflow.Transition(status, workflow.Start)
		if err != nil {
			return unchanged, err
		}
	}
	if frontier == len(ordered) {
		changes.Status, err = workflow.Transition(changes.Status, workflow.Succeed)
	} else {
		next := ordered[frontier]
		switch next.State.Status {
		case task.Failed:
			changes.Status, err = workflow.Transition(changes.Status, workflow.Fail)
		case task.Pending:
			next.State, err = task.Transition(next.State, task.Activate)
			changes.Tasks = []TaskSnapshot{next}
		}
	}
	if err != nil {
		return unchanged, err
	}
	return changes, nil
}

func orderedSnapshots(definitions []workflow.TaskDefinition, tasks []TaskSnapshot) ([]TaskSnapshot, error) {
	if len(tasks) != len(definitions) {
		return nil, ErrInvalidState
	}
	byKey := make(map[string]TaskSnapshot, len(tasks))
	for _, snapshot := range tasks {
		if _, exists := byKey[snapshot.Key]; exists {
			return nil, ErrInvalidState
		}
		if err := snapshot.State.Validate(); err != nil {
			return nil, ErrInvalidState
		}
		byKey[snapshot.Key] = snapshot
	}
	successors := make(map[string]int, len(definitions))
	root := 0
	for i, d := range definitions {
		snapshot, exists := byKey[d.ID]
		if !exists || snapshot.State.MaxAttemptCount != d.MaxAttempts {
			return nil, ErrInvalidState
		}
		if len(d.DependsOn) == 0 {
			root = i
		} else {
			successors[d.DependsOn[0]] = i
		}
	}
	ordered := make([]TaskSnapshot, 0, len(tasks))
	// ValidateSequence has already proved a complete chain; declaration order is irrelevant.
	for index := root; ; {
		key := definitions[index].ID
		ordered = append(ordered, byKey[key])
		next, exists := successors[key]
		if !exists {
			break
		}
		index = next
	}
	return ordered, nil
}
