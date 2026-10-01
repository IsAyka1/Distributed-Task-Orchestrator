package workflow

import "slices"

const TaskTypeActivity = "activity"

type DefinitionSpec struct {
	Provider string
	Name     string
	Version  int64
	Tasks    []TaskSpec
}

type TaskSpec struct {
	ID          string
	Type        string
	DependsOn   []string
	MaxAttempts *int64
}

type TaskDefinition struct {
	ID          string
	Type        string
	DependsOn   []string
	MaxAttempts int32
}

type Definition struct {
	provider string
	name     string
	version  int64
	tasks    []TaskDefinition
}

func NewDefinition(spec DefinitionSpec) (*Definition, error) {
	if err := validateDefinition(spec); err != nil {
		return nil, err
	}
	definition := &Definition{provider: spec.Provider, name: spec.Name, version: spec.Version}
	definition.tasks = make([]TaskDefinition, len(spec.Tasks))
	for i, task := range spec.Tasks {
		limit := int32(1)
		if task.MaxAttempts != nil {
			limit = int32(*task.MaxAttempts)
		}
		definition.tasks[i] = TaskDefinition{
			ID:          task.ID,
			Type:        task.Type,
			DependsOn:   slices.Clone(task.DependsOn),
			MaxAttempts: limit,
		}
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
