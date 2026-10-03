package workflows

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/definitions"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/workflow"
)

const definitionID = "00112233-4455-6677-8899-aabbccddeeff"

type startFake struct {
	EvaluationTransaction
	definition                   definitions.PublishedDefinition
	stage                        string
	failure                      error
	begun, committed, rolledBack bool
	request                      StartRequest
	tasks                        []TaskStart
	wakeup                       StartedWorkflow
}

func (f *startFake) fail(stage string) error {
	if f.stage == stage {
		return f.failure
	}
	return nil
}
func (f *startFake) FindByID(context.Context, string) (definitions.PublishedDefinition, error) {
	return f.definition, f.fail("find")
}
func (f *startFake) Begin(context.Context) (Transaction, error) {
	f.begun = true
	return f, f.fail("begin")
}
func (f *startFake) InsertWorkflow(_ context.Context, request StartRequest) (StartedWorkflow, error) {
	f.request = request
	return StartedWorkflow{ID: "run", Status: workflow.Pending, CreatedAt: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)}, f.fail("run")
}
func (f *startFake) InsertTask(_ context.Context, task TaskStart) (StartedTask, error) {
	f.tasks = append(f.tasks, task)
	return StartedTask{}, f.fail("task")
}
func (f *startFake) Enqueue(_ context.Context, run StartedWorkflow) (Wakeup, error) {
	f.wakeup = run
	return Wakeup{WorkflowID: run.ID, CreatedAt: run.CreatedAt}, f.fail("wakeup")
}
func (f *startFake) Commit() error   { f.committed = true; return f.fail("commit") }
func (f *startFake) Rollback() error { f.rolledBack = true; return nil }

func setup(t *testing.T, tasks []workflow.TaskSpec) (*startFake, Service) {
	t.Helper()
	d, err := definitions.ValidatePublication(workflow.DefinitionSpec{Provider: "demo", Name: "sequence", Version: 1, Tasks: tasks})
	if err != nil {
		t.Fatal(err)
	}
	f := &startFake{definition: definitions.PublishedDefinition{ID: definitionID, Definition: d}, failure: errors.New("injected")}
	return f, Service{Definitions: f, Repository: f}
}

func TestStartUsesDependencyRootAndCommitsAfterWakeup(t *testing.T) {
	f, service := setup(t, []workflow.TaskSpec{
		{ID: "B", Type: workflow.TaskTypeActivity, DependsOn: []string{"A"}},
		{ID: "A", Type: workflow.TaskTypeActivity},
	})
	input := workflow.Payload{Value: []byte(`{"n":1}`)}
	got, err := service.StartWorkflow(context.Background(), StartRequest{DefinitionID: definitionID, Input: input})
	if err != nil || got.ID != "run" || !f.committed || f.wakeup != got {
		t.Fatalf("got=%+v err=%v fake=%+v", got, err, f)
	}
	if string(f.tasks[0].Input.Value) != "null" || string(f.tasks[1].Input.Value) != `{"n":1}` {
		t.Fatal(f.tasks)
	}
	for _, task := range f.tasks {
		if task.WorkflowID != got.ID || task.CreatedAt != got.CreatedAt || task.MaxAttempts != 1 {
			t.Fatal(task)
		}
	}
	input.Value[0] = '['
	if string(f.request.Input.Value) != `{"n":1}` {
		t.Fatal("retained caller payload")
	}
}

func TestStartRejectsUnsupportedBeforeTransaction(t *testing.T) {
	limit := int64(2)
	for _, tc := range []struct {
		name  string
		tasks []workflow.TaskSpec
		want  error
	}{
		{"DAG", []workflow.TaskSpec{{ID: "A", Type: workflow.TaskTypeActivity}, {ID: "B", Type: workflow.TaskTypeActivity}}, workflow.ErrUnsupportedWorkflow},
		{"retry", []workflow.TaskSpec{{ID: "A", Type: workflow.TaskTypeActivity, MaxAttempts: &limit}}, ErrUnsupportedPolicy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, s := setup(t, tc.tasks)
			_, err := s.StartWorkflow(context.Background(), StartRequest{DefinitionID: definitionID})
			if !errors.Is(err, tc.want) || f.begun {
				t.Fatalf("err=%v begun=%v", err, f.begun)
			}
		})
	}
}

func TestStartErrorsReturnNoRunAndRollBack(t *testing.T) {
	for _, stage := range []string{"find", "begin", "run", "task", "wakeup", "commit"} {
		t.Run(stage, func(t *testing.T) {
			f, s := setup(t, []workflow.TaskSpec{{ID: "A", Type: workflow.TaskTypeActivity}})
			f.stage = stage
			got, err := s.StartWorkflow(context.Background(), StartRequest{DefinitionID: definitionID})
			if !errors.Is(err, f.failure) || got != (StartedWorkflow{}) {
				t.Fatalf("got=%+v err=%v", got, err)
			}
			if stage != "find" && stage != "begin" && !f.rolledBack {
				t.Fatal("rollback omitted")
			}
		})
	}
}

func TestStartValidatesRequestBeforeStorage(t *testing.T) {
	for _, request := range []StartRequest{
		{DefinitionID: definitionID, Input: workflow.Payload{Value: []byte("{")}},
		{DefinitionID: definitionID, Input: workflow.Payload{Value: []byte{'"', 0xff, '"'}}},
	} {
		_, err := (Service{}).StartWorkflow(context.Background(), request)
		if !errors.Is(err, ErrInvalidRequest) {
			t.Fatal(err)
		}
	}
}

func TestOmittedInputIsJSONNull(t *testing.T) {
	f, s := setup(t, []workflow.TaskSpec{{ID: "A", Type: workflow.TaskTypeActivity}})
	if _, err := s.StartWorkflow(context.Background(), StartRequest{DefinitionID: definitionID}); err != nil {
		t.Fatal(err)
	}
	if string(f.request.Input.Value) != "null" || string(f.tasks[0].Input.Value) != "null" {
		t.Fatal(f.request, f.tasks)
	}
}

func TestStartPassesDefinitionIDToRepository(t *testing.T) {
	f, s := setup(t, []workflow.TaskSpec{{ID: "A", Type: workflow.TaskTypeActivity}})
	id := "00112233445566778899aabbccddeeff"
	if _, err := s.StartWorkflow(context.Background(), StartRequest{DefinitionID: id}); err != nil {
		t.Fatal(err)
	}
	if f.request.DefinitionID != id {
		t.Fatal("definition ID changed")
	}
}

func TestStartRejectsInvalidDefinitionIDBeforeTransaction(t *testing.T) {
	f, s := setup(t, []workflow.TaskSpec{{ID: "A", Type: workflow.TaskTypeActivity}})
	f.stage, f.failure = "find", definitions.ErrInvalidDefinitionID
	_, err := s.StartWorkflow(context.Background(), StartRequest{DefinitionID: "invalid"})
	if !errors.Is(err, ErrInvalidRequest) || f.begun {
		t.Fatalf("err=%v begun=%v", err, f.begun)
	}
}
