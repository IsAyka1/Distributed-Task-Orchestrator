package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/workflows"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/task"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/workflow"
)

type inspectorFake struct {
	run     workflows.WorkflowDetails
	tasks   []workflows.TaskDetails
	err     error
	ids     []string
	entered chan struct{}
}

func (f *inspectorFake) read(ctx context.Context, id string) error {
	f.ids = append(f.ids, id)
	if f.entered != nil {
		close(f.entered)
		<-ctx.Done()
		return ctx.Err()
	}
	return f.err
}
func (f *inspectorFake) GetWorkflow(ctx context.Context, id string) (workflows.WorkflowDetails, error) {
	return f.run, f.read(ctx, id)
}
func (f *inspectorFake) ListTasks(ctx context.Context, id string) ([]workflows.TaskDetails, error) {
	return f.tasks, f.read(ctx, id)
}
func inspectSetup() (*fixture, *inspectorFake) {
	f := setup()
	inspector := &inspectorFake{run: workflows.WorkflowDetails{ID: "run", DefinitionID: "definition", Status: workflow.Running, Version: 7,
		Input: workflow.Payload{Value: json.RawMessage(`{"value":1}`)}, Output: workflow.Payload{Value: json.RawMessage(`null`)}, CreatedAt: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)}}
	f.handler.Inspection = inspector
	return f, inspector
}
func (f *fixture) get(path string) *httptest.ResponseRecorder {
	r := httptest.NewRecorder()
	f.handler.ServeHTTP(r, httptest.NewRequest(http.MethodGet, path, nil))
	return r
}
func TestWorkflowInspectionResponse(t *testing.T) {
	f, s := inspectSetup()
	started := s.run.CreatedAt.In(time.FixedZone("offset", 3*60*60))
	s.run.StartedAt = &started
	r := f.get("/workflows/run")
	want := `{"id":"run","definition_id":"definition","status":"RUNNING","input":{"value":1},"output":null,"version":7,"created_at":"2030-01-02T03:04:05Z","started_at":"2030-01-02T03:04:05Z","finished_at":null}`
	if r.Code != 200 || strings.TrimSpace(r.Body.String()) != want || r.Header().Get("Content-Type") != "application/json" {
		t.Fatal(r.Code, r.Body.String())
	}
	if len(s.ids) != 1 || s.ids[0] != "run" {
		t.Fatal(s.ids)
	}
}
func TestTaskInspectionResponse(t *testing.T) {
	f, s := inspectSetup()
	token := "attempt"
	s.tasks = []workflows.TaskDetails{{ID: "task", Key: "zeta", Status: task.Running, AttemptCount: 1, MaxAttemptCount: 1, CurrentAttemptID: &token, Input: workflow.Payload{Value: json.RawMessage(`[1,true]`)}, Output: workflow.Payload{Value: json.RawMessage(`null`)}}}
	r := f.get("/workflows/run/tasks")
	want := `{"tasks":[{"id":"task","task_key":"zeta","status":"RUNNING","input":[1,true],"output":null,"attempt_count":1,"max_attempt_count":1,"current_attempt_id":"attempt"}]}`
	if r.Code != 200 || strings.TrimSpace(r.Body.String()) != want {
		t.Fatal(r.Code, r.Body.String())
	}
	s.tasks[0].CurrentAttemptID = nil
	if !strings.Contains(f.get("/workflows/run/tasks").Body.String(), `"current_attempt_id":null`) {
		t.Fatal("missing nullable attempt")
	}
}
func TestInspectionErrors(t *testing.T) {
	for _, suffix := range []string{"", "/tasks"} {
		for _, tc := range []struct {
			err    error
			status int
			code   string
		}{
			{workflows.ErrWorkflowNotFound, 404, "workflow_not_found"},
			{workflows.ErrInvalidRequest, 400, "invalid_request"},
			{context.DeadlineExceeded, 503, "storage_unavailable"},
			{errors.New("secret database detail"), 500, "internal_error"},
		} {
			f, s := inspectSetup()
			s.err = tc.err
			assertError(t, f.get("/workflows/request-id"+suffix), tc.status, tc.code)
			if len(s.ids) != 1 || s.ids[0] != "request-id" {
				t.Fatal(s.ids)
			}
		}
	}
}
func TestInspectionRouteAndMethod(t *testing.T) {
	f, s := inspectSetup()
	for _, path := range []string{"/workflows/", "/workflows//tasks", "/workflows/id/", "/workflows/id/unknown", "/workflows/id/tasks/extra"} {
		assertError(t, f.get(path), 404, "not_found")
	}
	for _, path := range []string{"/workflows/id", "/workflows/id/tasks"} {
		r := f.request(path, `{}`, "application/json")
		assertError(t, r, 405, "method_not_allowed")
		if r.Header().Get("Allow") != "GET" {
			t.Fatal(r.Header())
		}
	}
	if len(s.ids) != 0 {
		t.Fatal(s.ids)
	}
}
func TestInspectionTimeout(t *testing.T) {
	for _, suffix := range []string{"", "/tasks"} {
		f, s := inspectSetup()
		s.entered = make(chan struct{})
		done := make(chan *httptest.ResponseRecorder, 1)
		go func() { done <- f.get("/workflows/id" + suffix) }()
		<-s.entered
		f.clock.Advance(time.Second)
		assertError(t, <-done, 503, "storage_unavailable")
	}
}
