package workflows

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/engine"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/definitions"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/task"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/workflow"
)

type evaluationFake struct {
	Transaction
	run         EvaluationRun
	definition  definitions.PublishedDefinition
	tasks       []EvaluationTask
	wakeup      bool
	now         time.Time
	calls       []string
	failAt      string
	activations []Activation
	updates     []EvaluationRun
}

var evaluationFailure = errors.New("injected evaluation failure")

func (f *evaluationFake) call(name string) error {
	f.calls = append(f.calls, name)
	if name == f.failAt {
		return evaluationFailure
	}
	return nil
}
func (f *evaluationFake) Begin(context.Context) (Transaction, error) { return f, f.call("begin") }
func (f *evaluationFake) LockWorkflow(context.Context, string) (EvaluationRun, error) {
	return f.run, f.call("workflow")
}
func (f *evaluationFake) LockWakeup(context.Context, string) (bool, error) {
	return f.wakeup, f.call("wakeup")
}
func (f *evaluationFake) FindDefinition(context.Context, string) (definitions.PublishedDefinition, error) {
	return f.definition, f.call("definition")
}
func (f *evaluationFake) LockTasks(context.Context, string) ([]EvaluationTask, error) {
	return f.tasks, f.call("tasks")
}
func (f *evaluationFake) Now(context.Context) (time.Time, error) { return f.now, f.call("now") }
func (f *evaluationFake) Activate(_ context.Context, a Activation) error {
	f.activations = append(f.activations, a)
	return f.call("activate")
}
func (f *evaluationFake) UpdateEvaluation(_ context.Context, r EvaluationRun, at time.Time) error {
	if at != f.now {
		panic("incorrect time")
	}
	f.updates = append(f.updates, r)
	return f.call("update")
}
func (f *evaluationFake) DeleteWakeup(context.Context, string) error { return f.call("delete") }
func (f *evaluationFake) Commit() error                              { return f.call("commit") }
func (f *evaluationFake) Rollback() error                            { return f.call("rollback") }
func evaluationSetup(t *testing.T) (*evaluationFake, Service) {
	t.Helper()
	d, err := definitions.ValidatePublication(workflow.DefinitionSpec{Provider: "demo", Name: "sequence", Version: 1, Tasks: []workflow.TaskSpec{
		{ID: "B", Type: workflow.TaskTypeActivity, DependsOn: []string{"A"}}, {ID: "A", Type: workflow.TaskTypeActivity},
	}})
	if err != nil {
		t.Fatal(err)
	}
	f := &evaluationFake{run: EvaluationRun{ID: "run", DefinitionID: definitionID, Status: workflow.Pending, Output: workflow.Payload{Value: []byte("null")}}, definition: definitions.PublishedDefinition{Definition: d}, wakeup: true, now: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)}
	for _, key := range []string{"B", "A"} {
		f.tasks = append(f.tasks, EvaluationTask{ID: key, Snapshot: engine.TaskSnapshot{Key: key, State: task.State{Status: task.Pending, MaxAttemptCount: 1}}, Output: workflow.Payload{Value: []byte("null")}})
	}
	return f, Service{Repository: f}
}
func completeSnapshot(t *EvaluationTask, status task.Status, output string) {
	attempt := task.AttemptSucceeded
	if status == task.Failed {
		attempt = task.AttemptFailed
	}
	t.Snapshot.State = task.State{Status: status, MaxAttemptCount: 1, AttemptCount: 1, LastAttempt: task.Attempt{Number: 1, Status: attempt}}
	t.Output.Value = []byte(output)
}
func TestEvaluationStartsUnderLocks(t *testing.T) {
	f, s := evaluationSetup(t)
	if err := s.EvaluateWorkflow(context.Background(), "run"); err != nil {
		t.Fatal(err)
	}
	if len(f.activations) != 1 || f.activations[0].ID != "A" || f.activations[0].Input.Value != nil || f.activations[0].At != f.now {
		t.Fatal(f.activations)
	}
	if len(f.updates) != 1 || f.updates[0].Status != workflow.Running {
		t.Fatal(f.updates)
	}
	want := []string{"begin", "workflow", "wakeup", "definition", "tasks", "now", "activate", "update", "delete", "commit", "rollback"}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatal(f.calls)
	}
}
func TestEvaluationPropagatesWholeOutput(t *testing.T) {
	for _, output := range []string{`{"n":2}`, `[3]`, `null`} {
		t.Run(output, func(t *testing.T) {
			f, s := evaluationSetup(t)
			f.run.Status = workflow.Running
			completeSnapshot(&f.tasks[1], task.Succeeded, output)
			if err := s.EvaluateWorkflow(context.Background(), "run"); err != nil {
				t.Fatal(err)
			}
			if len(f.activations) != 1 || f.activations[0].ID != "B" || string(f.activations[0].Input.Value) != output {
				t.Fatal(f.activations)
			}
			if len(f.updates) != 1 || f.updates[0].Status != workflow.Running {
				t.Fatal(f.updates)
			}
		})
	}
}
func TestEvaluationPersistsTerminalOutcomes(t *testing.T) {
	for _, status := range []task.Status{task.Succeeded, task.Failed} {
		t.Run(string(status), func(t *testing.T) {
			f, s := evaluationSetup(t)
			f.run.Status = workflow.Running
			completeSnapshot(&f.tasks[1], task.Succeeded, `[1]`)
			completeSnapshot(&f.tasks[0], status, `{"final":true}`)
			if err := s.EvaluateWorkflow(context.Background(), "run"); err != nil {
				t.Fatal(err)
			}
			if len(f.updates) != 1 || len(f.activations) != 0 {
				t.Fatal(f.updates, f.activations)
			}
			want := workflow.Succeeded
			output := `{"final":true}`
			if status == task.Failed {
				want = workflow.Failed
				output = "null"
			}
			if f.updates[0].Status != want || string(f.updates[0].Output.Value) != output {
				t.Fatal(f.updates)
			}
		})
	}
}
func TestEvaluationNoops(t *testing.T) {
	for _, scenario := range []string{"no wakeup", "terminal success", "terminal failure", "ready", "running"} {
		t.Run(scenario, func(t *testing.T) {
			f, s := evaluationSetup(t)
			switch scenario {
			case "no wakeup":
				f.wakeup = false
			case "terminal success":
				f.run.Status = workflow.Succeeded
				f.definition.Definition = nil
				f.tasks = nil
			case "terminal failure":
				f.run.Status = workflow.Failed
				f.definition.Definition = nil
				f.tasks = nil
			case "ready":
				f.run.Status = workflow.Running
				f.tasks[1].Snapshot.State.Status = task.Ready
			case "running":
				f.run.Status = workflow.Running
				f.tasks[1].Snapshot.State = task.State{Status: task.Running, MaxAttemptCount: 1, AttemptCount: 1, LastAttempt: task.Attempt{Number: 1, Status: task.AttemptRunning}}
			}
			if err := s.EvaluateWorkflow(context.Background(), "run"); err != nil {
				t.Fatal(err)
			}
			if len(f.updates) != 0 || len(f.activations) != 0 {
				t.Fatal(f.updates, f.activations)
			}
			for _, c := range f.calls {
				if c == "now" {
					t.Fatal("no-op sampled time")
				}
			}
		})
	}
}
func TestEvaluationErrorsRollback(t *testing.T) {
	for _, stage := range []string{"begin", "workflow", "wakeup", "definition", "tasks", "now", "activate", "update", "delete", "commit"} {
		t.Run(stage, func(t *testing.T) {
			f, s := evaluationSetup(t)
			f.failAt = stage
			if err := s.EvaluateWorkflow(context.Background(), "run"); !errors.Is(err, evaluationFailure) {
				t.Fatal(err)
			}
			if stage != "begin" && f.calls[len(f.calls)-1] != "rollback" {
				t.Fatal(f.calls)
			}
		})
	}
}
func TestEvaluationRejectsInconsistentSnapshot(t *testing.T) {
	f, s := evaluationSetup(t)
	f.tasks = f.tasks[:1]
	if err := s.EvaluateWorkflow(context.Background(), "run"); !errors.Is(err, engine.ErrInvalidState) {
		t.Fatal(err)
	}
	for _, c := range f.calls {
		if c == "delete" || c == "update" || c == "activate" {
			t.Fatal(f.calls)
		}
	}
}
