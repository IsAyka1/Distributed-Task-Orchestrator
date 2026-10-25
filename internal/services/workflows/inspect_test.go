package workflows

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/definitions"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/task"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/workflow"
)

type inspectionFake struct {
	run        WorkflowDetails
	tasks      []TaskDetails
	definition definitions.PublishedDefinition
	fail       string
	err        error
	calls      []string
}

func (f *inspectionFake) step(name string) error {
	f.calls = append(f.calls, name)
	if f.fail == name {
		return f.err
	}
	return nil
}
func (f *inspectionFake) BeginRead(context.Context) (ReadTransaction, error) {
	return f, f.step("begin")
}
func (f *inspectionFake) GetWorkflow(context.Context, string) (WorkflowDetails, error) {
	return f.run, f.step("run")
}
func (f *inspectionFake) ListTasks(context.Context, string) ([]TaskDetails, error) {
	return f.tasks, f.step("tasks")
}
func (f *inspectionFake) FindDefinition(context.Context, string) (definitions.PublishedDefinition, error) {
	return f.definition, f.step("definition")
}
func (f *inspectionFake) Commit() error   { return f.step("commit") }
func (f *inspectionFake) Rollback() error { return f.step("rollback") }
func inspectionSetup(t *testing.T) (*inspectionFake, Inspector) {
	t.Helper()
	d, err := workflow.NewDefinition(workflow.DefinitionSpec{Provider: "test", Name: "sequence", Version: 1, Tasks: []workflow.TaskSpec{
		{ID: "last", Type: workflow.TaskTypeActivity, DependsOn: []string{"alpha"}},
		{ID: "zeta", Type: workflow.TaskTypeActivity},
		{ID: "alpha", Type: workflow.TaskTypeActivity, DependsOn: []string{"zeta"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	f := &inspectionFake{run: WorkflowDetails{ID: "run", DefinitionID: "definition", Status: workflow.Running, Version: 3}, tasks: []TaskDetails{{Key: "alpha", Status: task.Ready}, {Key: "last", Status: task.Pending}, {Key: "zeta", Status: task.Succeeded}}, definition: definitions.PublishedDefinition{Definition: d}, err: errors.New("storage failure")}
	return f, Inspector{Repository: f}
}
func TestInspectionSnapshot(t *testing.T) {
	f, s := inspectionSetup(t)
	run, err := s.GetWorkflow(context.Background(), "run")
	if err != nil || run.ID != "run" || run.Version != 3 {
		t.Fatal(run, err)
	}
	if !reflect.DeepEqual(f.calls, []string{"begin", "run", "commit", "rollback"}) {
		t.Fatal(f.calls)
	}
	f.calls = nil
	tasks, err := s.ListTasks(context.Background(), "run")
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{}
	for _, v := range tasks {
		keys = append(keys, v.Key)
	}
	if !reflect.DeepEqual(keys, []string{"zeta", "alpha", "last"}) {
		t.Fatal(keys)
	}
	if !reflect.DeepEqual(f.calls, []string{"begin", "run", "definition", "tasks", "commit", "rollback"}) {
		t.Fatal(f.calls)
	}
}
func TestInspectionFailuresReturnNoResult(t *testing.T) {
	for _, phase := range []string{"begin", "run", "definition", "tasks", "commit"} {
		t.Run(phase, func(t *testing.T) {
			f, s := inspectionSetup(t)
			f.fail = phase
			got, err := s.ListTasks(context.Background(), "run")
			if !errors.Is(err, f.err) || got != nil {
				t.Fatal(got, err)
			}
			if phase != "begin" && f.calls[len(f.calls)-1] != "rollback" {
				t.Fatal(f.calls)
			}
		})
	}
	for _, phase := range []string{"begin", "run", "commit"} {
		f, s := inspectionSetup(t)
		f.fail = phase
		got, err := s.GetWorkflow(context.Background(), "run")
		if !errors.Is(err, f.err) || got.ID != "" {
			t.Fatal(got, err)
		}
	}
}
func TestInspectionRejectsInconsistentTasks(t *testing.T) {
	for _, keys := range [][]string{{}, {"zeta", "alpha"}, {"zeta", "alpha", "unknown"}, {"zeta", "zeta", "last"}, {"zeta", "alpha", "last", "extra"}} {
		f, s := inspectionSetup(t)
		f.tasks = nil
		for _, key := range keys {
			f.tasks = append(f.tasks, TaskDetails{Key: key, Status: task.Pending})
		}
		if got, err := s.ListTasks(context.Background(), "run"); err == nil || got != nil {
			t.Fatal(keys, got, err)
		}
		for _, call := range f.calls {
			if call == "commit" {
				t.Fatal("invalid snapshot committed")
			}
		}
	}
}

func TestInspectionStoredDefinitionErrorsAreInternal(t *testing.T) {
	for _, err := range []error{definitions.ErrDefinitionNotFound, definitions.ErrInvalidDefinitionID, workflow.ErrInvalidDefinition} {
		f, s := inspectionSetup(t)
		f.fail = "definition"
		f.err = err
		result, got := s.ListTasks(context.Background(), "run")
		if !errors.Is(got, errInvalidInspection) || result != nil {
			t.Fatal(result, got)
		}
	}
}

func TestInspectionRejectsUnknownWorkflowStatus(t *testing.T) {
	for _, status := range []workflow.Status{"", "UNKNOWN", "running", "CANCELLED"} {
		for _, includeTasks := range []bool{false, true} {
			f, s := inspectionSetup(t)
			f.run.Status = status
			if includeTasks {
				got, err := s.ListTasks(context.Background(), "run")
				if !errors.Is(err, errInvalidInspection) || got != nil {
					t.Fatalf("status %q: tasks=%v err=%v", status, got, err)
				}
			} else {
				got, err := s.GetWorkflow(context.Background(), "run")
				if !errors.Is(err, errInvalidInspection) || !reflect.DeepEqual(got, WorkflowDetails{}) {
					t.Fatalf("status %q: run=%v err=%v", status, got, err)
				}
			}
			if !reflect.DeepEqual(f.calls, []string{"begin", "run", "rollback"}) {
				t.Fatal(f.calls)
			}
		}
	}
}

func TestInspectionRejectsUnknownTaskStatus(t *testing.T) {
	for _, status := range []task.Status{"", "UNKNOWN", "ready", "RETRY_WAIT"} {
		for i := 0; i < 3; i++ {
			f, s := inspectionSetup(t)
			f.tasks[i].Status = status
			got, err := s.ListTasks(context.Background(), "run")
			if !errors.Is(err, errInvalidInspection) || got != nil {
				t.Fatalf("task %d status %q: tasks=%v err=%v", i, status, got, err)
			}
			if !reflect.DeepEqual(f.calls, []string{"begin", "run", "definition", "tasks", "rollback"}) {
				t.Fatal(f.calls)
			}
		}
	}
}

func TestInspectionAcceptsKnownStatuses(t *testing.T) {
	for _, status := range []workflow.Status{workflow.Pending, workflow.Running, workflow.Succeeded, workflow.Failed} {
		f, s := inspectionSetup(t)
		f.run.Status = status
		if got, err := s.GetWorkflow(context.Background(), "run"); err != nil || got.Status != status {
			t.Fatal(got, err)
		}
	}
	for _, status := range []task.Status{task.Pending, task.Ready, task.Running, task.Succeeded, task.Failed} {
		f, s := inspectionSetup(t)
		f.tasks[0].Status = status
		if got, err := s.ListTasks(context.Background(), "run"); err != nil || got[1].Status != status {
			t.Fatal(got, err)
		}
	}
}
