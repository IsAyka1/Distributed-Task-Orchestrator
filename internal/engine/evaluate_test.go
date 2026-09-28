package engine_test

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/engine"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/task"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/workflow"
)

func definition(t *testing.T, tasks ...workflow.TaskSpec) workflow.Definition {
	t.Helper()
	d, err := workflow.NewDefinition(workflow.DefinitionSpec{Provider: "test", Name: "sequence", Version: 1, Tasks: tasks})
	if err != nil {
		t.Fatal(err)
	}
	return *d
}
func chain(t *testing.T) workflow.Definition {
	return definition(t, workflow.TaskSpec{ID: "C", DependsOn: []string{"B"}}, workflow.TaskSpec{ID: "A"}, workflow.TaskSpec{ID: "B", DependsOn: []string{"A"}})
}
func snapshot(key string, status task.Status) engine.TaskSnapshot {
	s := task.State{Status: status, MaxAttemptCount: 1}
	switch status {
	case task.Running:
		s.AttemptCount = 1
		s.LastAttempt = task.Attempt{Number: 1, Status: task.AttemptRunning}
	case task.Succeeded:
		s.AttemptCount = 1
		s.LastAttempt = task.Attempt{Number: 1, Status: task.AttemptSucceeded}
	case task.Failed:
		s.AttemptCount = 1
		s.LastAttempt = task.Attempt{Number: 1, Status: task.AttemptFailed}
	}
	return engine.TaskSnapshot{Key: key, State: s}
}
func TestEvaluateSequence(t *testing.T) {
	d := chain(t)
	tests := []struct {
		name    string
		status  workflow.Status
		a, b, c task.Status
		want    workflow.Status
		ready   string
	}{
		{"start", workflow.Pending, task.Pending, task.Pending, task.Pending, workflow.Running, "A"},
		{"root ready", workflow.Running, task.Ready, task.Pending, task.Pending, workflow.Running, ""},
		{"root running", workflow.Running, task.Running, task.Pending, task.Pending, workflow.Running, ""},
		{"root succeeded", workflow.Running, task.Succeeded, task.Pending, task.Pending, workflow.Running, "B"},
		{"middle ready", workflow.Running, task.Succeeded, task.Ready, task.Pending, workflow.Running, ""},
		{"middle running", workflow.Running, task.Succeeded, task.Running, task.Pending, workflow.Running, ""},
		{"middle succeeded", workflow.Running, task.Succeeded, task.Succeeded, task.Pending, workflow.Running, "C"},
		{"last running", workflow.Running, task.Succeeded, task.Succeeded, task.Running, workflow.Running, ""},
		{"all succeeded", workflow.Running, task.Succeeded, task.Succeeded, task.Succeeded, workflow.Succeeded, ""},
		{"root failed", workflow.Running, task.Failed, task.Pending, task.Pending, workflow.Failed, ""},
		{"middle failed", workflow.Running, task.Succeeded, task.Failed, task.Pending, workflow.Failed, ""},
		{"last failed", workflow.Running, task.Succeeded, task.Succeeded, task.Failed, workflow.Failed, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := []engine.TaskSnapshot{snapshot("B", tt.b), snapshot("C", tt.c), snapshot("A", tt.a)}
			before := slices.Clone(input)
			got, err := engine.Evaluate(d, tt.status, input)
			want := engine.Changes{Status: tt.want}
			if tt.ready != "" {
				want.Tasks = []engine.TaskSnapshot{snapshot(tt.ready, task.Ready)}
			}
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("Evaluate = %+v, %v; want %+v", got, err, want)
			}
			if !reflect.DeepEqual(input, before) {
				t.Fatal("input mutated")
			}
			// Persist only the proposed changes; another evaluation must be a no-op.
			for _, change := range got.Tasks {
				for i := range input {
					if input[i].Key == change.Key {
						input[i] = change
					}
				}
			}
			again, err := engine.Evaluate(d, got.Status, input)
			if err != nil || again.Status != got.Status || len(again.Tasks) != 0 {
				t.Fatalf("repeated evaluation = %+v, %v", again, err)
			}
			if len(got.Tasks) > 0 {
				persisted := slices.Clone(input)
				got.Tasks[0].State.Status = task.Failed
				if !reflect.DeepEqual(input, persisted) {
					t.Fatal("result aliases input")
				}
			}
		})
	}
}
func TestInvalidSnapshots(t *testing.T) {
	tests := []struct {
		name   string
		status workflow.Status
		tasks  []engine.TaskSnapshot
	}{
		{"missing", workflow.Running, []engine.TaskSnapshot{snapshot("A", task.Pending)}},
		{"extra", workflow.Running, []engine.TaskSnapshot{snapshot("A", task.Pending), snapshot("B", task.Pending), snapshot("C", task.Pending), snapshot("D", task.Pending)}},
		{"duplicate", workflow.Running, []engine.TaskSnapshot{snapshot("A", task.Pending), snapshot("A", task.Pending), snapshot("C", task.Pending)}},
		{"unknown", workflow.Running, []engine.TaskSnapshot{snapshot("A", task.Pending), snapshot("B", task.Pending), snapshot("D", task.Pending)}},
		{"unknown status", workflow.Status("UNKNOWN"), []engine.TaskSnapshot{snapshot("A", task.Pending), snapshot("B", task.Pending), snapshot("C", task.Pending)}},
		{"pending with started task", workflow.Pending, []engine.TaskSnapshot{snapshot("A", task.Ready), snapshot("B", task.Pending), snapshot("C", task.Pending)}},
		{"success before dependency", workflow.Running, []engine.TaskSnapshot{snapshot("A", task.Pending), snapshot("B", task.Succeeded), snapshot("C", task.Pending)}},
		{"two active", workflow.Running, []engine.TaskSnapshot{snapshot("A", task.Running), snapshot("B", task.Ready), snapshot("C", task.Pending)}},
		{"started after failure", workflow.Running, []engine.TaskSnapshot{snapshot("A", task.Failed), snapshot("B", task.Running), snapshot("C", task.Pending)}},
		{"unknown task status", workflow.Running, []engine.TaskSnapshot{snapshot("A", task.Status("UNKNOWN")), snapshot("B", task.Pending), snapshot("C", task.Pending)}},
	}
	bad := []engine.TaskSnapshot{snapshot("A", task.Succeeded), snapshot("B", task.Pending), snapshot("C", task.Pending)}
	bad[0].State.LastAttempt.Status = task.AttemptRunning
	tests = append(tests, struct {
		name   string
		status workflow.Status
		tasks  []engine.TaskSnapshot
	}{"inconsistent attempt", workflow.Running, bad})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := slices.Clone(tt.tasks)
			got, err := engine.Evaluate(chain(t), tt.status, tt.tasks)
			if !errors.Is(err, engine.ErrInvalidState) || got.Status != tt.status || len(got.Tasks) != 0 {
				t.Fatalf("got %+v, %v", got, err)
			}
			if !reflect.DeepEqual(tt.tasks, before) {
				t.Fatal("rejection mutated input")
			}
		})
	}
}
func TestDefinitionGuardsAndTerminalRuns(t *testing.T) {
	branch := definition(t, workflow.TaskSpec{ID: "A"}, workflow.TaskSpec{ID: "B", DependsOn: []string{"A"}}, workflow.TaskSpec{ID: "C", DependsOn: []string{"A"}})
	limit := int64(2)
	retry := definition(t, workflow.TaskSpec{ID: "A", MaxAttempts: &limit})
	for _, tt := range []struct {
		d    workflow.Definition
		want error
	}{
		{workflow.Definition{}, workflow.ErrInvalidDefinition}, {branch, workflow.ErrUnsupportedWorkflow}, {retry, task.ErrUnsupportedPolicy},
	} {
		got, err := engine.Evaluate(tt.d, workflow.Pending, []engine.TaskSnapshot{snapshot("A", task.Pending)})
		if !errors.Is(err, tt.want) || got.Status != workflow.Pending || len(got.Tasks) != 0 {
			t.Fatalf("got %+v, %v; want %v", got, err, tt.want)
		}
	}
	for _, status := range []workflow.Status{workflow.Succeeded, workflow.Failed} {
		got, err := engine.Evaluate(workflow.Definition{}, status, nil)
		if err != nil || got.Status != status || len(got.Tasks) != 0 {
			t.Fatalf("terminal evaluation = %+v, %v", got, err)
		}
	}
}
func TestSingleTaskWithEmptyKey(t *testing.T) {
	d := definition(t, workflow.TaskSpec{ID: ""})
	got, err := engine.Evaluate(d, workflow.Pending, []engine.TaskSnapshot{snapshot("", task.Pending)})
	if err != nil || len(got.Tasks) != 1 || got.Tasks[0].Key != "" || got.Tasks[0].State.Status != task.Ready {
		t.Fatalf("activation = %+v, %v", got, err)
	}
	got, err = engine.Evaluate(d, workflow.Running, []engine.TaskSnapshot{snapshot("", task.Succeeded)})
	if err != nil || got.Status != workflow.Succeeded {
		t.Fatalf("completion = %+v, %v", got, err)
	}
}

func TestSequenceLifecycle(t *testing.T) {
	d := chain(t)
	tasks := []engine.TaskSnapshot{snapshot("C", task.Pending), snapshot("B", task.Pending), snapshot("A", task.Pending)}
	status := workflow.Pending
	for _, key := range []string{"A", "B", "C"} {
		changes, err := engine.Evaluate(d, status, tasks)
		if err != nil || changes.Status != workflow.Running || len(changes.Tasks) != 1 || changes.Tasks[0].Key != key {
			t.Fatalf("activation for %q = %+v, %v", key, changes, err)
		}
		status = changes.Status
		for i := range tasks {
			if tasks[i].Key != key {
				continue
			}
			tasks[i] = changes.Tasks[0]
			for _, event := range []task.Event{task.Claim, task.Succeed} {
				tasks[i].State, err = task.Transition(tasks[i].State, event)
				if err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	changes, err := engine.Evaluate(d, status, tasks)
	if err != nil || changes.Status != workflow.Succeeded || len(changes.Tasks) != 0 {
		t.Fatalf("final evaluation = %+v, %v", changes, err)
	}
}
