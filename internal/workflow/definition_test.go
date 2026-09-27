package workflow_test

import (
	"errors"
	"math"
	"reflect"
	"strconv"
	"testing"

	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/workflow"
)

func task(id string, dependencies ...string) workflow.TaskSpec {
	return workflow.TaskSpec{ID: id, Type: "activity", DependsOn: dependencies}
}

func spec(tasks ...workflow.TaskSpec) workflow.DefinitionSpec {
	return workflow.DefinitionSpec{Provider: "demo", Name: "sequence", Version: 1, Tasks: tasks}
}

func TestDefinitionValidation(t *testing.T) {
	tests := []struct {
		name     string
		input    workflow.DefinitionSpec
		invalid  bool
		sequence bool
	}{
		{"single", spec(task("A")), false, true},
		{"chain out of declaration order", spec(task("C", "B"), task("A"), task("B", "A")), false, true},
		{"branch", spec(task("A"), task("B", "A"), task("C", "A")), false, false},
		{"join", spec(task("A"), task("B"), task("C", "A", "B")), false, false},
		{"diamond", spec(task("A"), task("B", "A"), task("C", "A"), task("D", "B", "C")), false, false},
		{"disconnected", spec(task("A"), task("B")), false, false},
		{"empty", spec(), true, false},
		{"duplicate IDs", spec(task("A"), task("A")), true, false},
		{"unknown dependency", spec(task("A", "missing")), true, false},
		{"duplicate dependency", spec(task("A"), task("B", "A", "A")), true, false},
		{"self loop", spec(task("A", "A")), true, false},
		{"cycle", spec(task("A", "B"), task("B", "A")), true, false},
		{"disconnected cycle", spec(task("root"), task("A", "B"), task("B", "C"), task("C", "A")), true, false},
		{"case sensitive IDs", spec(task("A"), task("a", "A")), false, true},
		{"IDs are not trimmed", spec(task(" A "), task("A", " A ")), false, true},
		{"case sensitive dependency", spec(task("A"), task("B", "a")), true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			definition, err := workflow.NewDefinition(tt.input)
			if tt.invalid {
				if !errors.Is(err, workflow.ErrInvalidDefinition) {
					t.Fatalf("error = %v", err)
				}
				if definition != nil {
					t.Fatal("invalid definition returned")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			err = definition.ValidateSequence()
			if tt.sequence && err != nil {
				t.Fatal(err)
			}
			if !tt.sequence && !errors.Is(err, workflow.ErrUnsupportedWorkflow) {
				t.Fatalf("sequence error = %v", err)
			}
		})
	}
}

func TestDefinitionFieldsAndPolicy(t *testing.T) {
	type policyCase struct {
		name     string
		change   func(*workflow.DefinitionSpec)
		invalid  bool
		attempts int32
	}
	tests := []policyCase{
		{"default", func(*workflow.DefinitionSpec) {}, false, 1},
		{"provider required", func(s *workflow.DefinitionSpec) { s.Provider = "" }, true, 0},
		{"name required", func(s *workflow.DefinitionSpec) { s.Name = "" }, true, 0},
		{"version zero", func(s *workflow.DefinitionSpec) { s.Version = 0 }, true, 0},
		{"version negative", func(s *workflow.DefinitionSpec) { s.Version = -1 }, true, 0},
		{"version gaps allowed", func(s *workflow.DefinitionSpec) { s.Version = 100 }, false, 1},
	}
	for _, value := range []int64{-1, 0, 1, 2, math.MaxInt32, math.MaxInt32 + 1} {
		tests = append(tests, policyCase{
			name:    "attempts " + strconv.FormatInt(value, 10),
			change:  func(s *workflow.DefinitionSpec) { s.Tasks[0].MaxAttempts = &value },
			invalid: value < 1 || value > math.MaxInt32, attempts: int32(value),
		})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := spec(task("A"))
			tt.change(&input)
			definition, err := workflow.NewDefinition(input)
			if tt.invalid {
				if !errors.Is(err, workflow.ErrInvalidDefinition) {
					t.Fatalf("error = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if definition.Provider() != input.Provider || definition.Name() != input.Name || definition.Version() != input.Version {
				t.Fatal("identity changed")
			}
			if got := definition.Tasks()[0].MaxAttempts; got != tt.attempts {
				t.Fatalf("max attempts = %d, want %d", got, tt.attempts)
			}
		})
	}
}

func TestDefinitionDefensiveCopy(t *testing.T) {
	limit := int64(2)
	input := spec(task("A"), task("B", "A"))
	input.Provider, input.Name = " Demo ", " Sequence "
	input.Tasks[1].MaxAttempts = &limit
	definition, err := workflow.NewDefinition(input)
	if err != nil {
		t.Fatal(err)
	}
	want := []workflow.TaskDefinition{
		{ID: "A", Type: "activity", MaxAttempts: 1},
		{ID: "B", Type: "activity", DependsOn: []string{"A"}, MaxAttempts: 2},
	}
	if input.Tasks[0].MaxAttempts != nil || limit != 2 {
		t.Fatal("constructor modified input policy")
	}
	input.Tasks[0].ID = "changed"
	input.Tasks[1].DependsOn[0] = "changed"
	limit = 0
	input.Tasks = append(input.Tasks, task("C"))
	output := definition.Tasks()
	output[0].ID = "changed again"
	output[1].DependsOn[0] = "changed again"
	output[1].MaxAttempts = 0
	if !reflect.DeepEqual(definition.Tasks(), want) {
		t.Fatal("definition mutated through input or output")
	}
	if definition.Provider() != " Demo " || definition.Name() != " Sequence " {
		t.Fatal("identity normalized")
	}
	if err := definition.ValidateSequence(); err != nil {
		t.Fatal(err)
	}
}

func TestZeroDefinitionCannotExecute(t *testing.T) {
	var definition workflow.Definition
	if !errors.Is(definition.ValidateSequence(), workflow.ErrInvalidDefinition) {
		t.Fatal("zero definition accepted")
	}
}
