package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/workflows"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/task"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/workflow"
)

type Inspector interface {
	GetWorkflow(context.Context, string) (workflows.WorkflowDetails, error)
	ListTasks(context.Context, string) ([]workflows.TaskDetails, error)
}
type WorkflowResponse struct {
	ID           string          `json:"id"`
	DefinitionID string          `json:"definition_id"`
	Status       workflow.Status `json:"status"`
	Input        json.RawMessage `json:"input"`
	Output       json.RawMessage `json:"output"`
	Version      int64           `json:"version"`
	CreatedAt    time.Time       `json:"created_at"`
	StartedAt    *time.Time      `json:"started_at"`
	FinishedAt   *time.Time      `json:"finished_at"`
}
type TaskResponse struct {
	ID               string          `json:"id"`
	Key              string          `json:"task_key"`
	Status           task.Status     `json:"status"`
	Input            json.RawMessage `json:"input"`
	Output           json.RawMessage `json:"output"`
	AttemptCount     int32           `json:"attempt_count"`
	MaxAttemptCount  int32           `json:"max_attempt_count"`
	CurrentAttemptID *string         `json:"current_attempt_id"`
}
type TasksResponse struct {
	Tasks []TaskResponse `json:"tasks"`
}

func utc(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	at := value.UTC()
	return &at
}
func (h Handler) serveInspection(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/workflows/"), "/")
	if parts[0] == "" || len(parts) > 2 || (len(parts) == 2 && parts[1] != "tasks") {
		writeError(w, 404, "not_found", "This endpoint does not exist.")
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, 405, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	ctx, cancel := h.Clock.WithTimeout(r.Context(), h.Timeout)
	defer cancel()
	if len(parts) == 1 {
		run, err := h.Inspection.GetWorkflow(ctx, parts[0])
		if err != nil {
			writeServiceError(ctx, w, err)
			return
		}
		writeJSON(w, 200, WorkflowResponse{ID: run.ID, DefinitionID: run.DefinitionID, Status: run.Status,
			Input: run.Input.Value, Output: run.Output.Value, Version: run.Version, CreatedAt: run.CreatedAt.UTC(),
			StartedAt: utc(run.StartedAt), FinishedAt: utc(run.FinishedAt)})
	} else {
		tasks, err := h.Inspection.ListTasks(ctx, parts[0])
		if err != nil {
			writeServiceError(ctx, w, err)
			return
		}
		response := TasksResponse{Tasks: make([]TaskResponse, 0, len(tasks))}
		for _, value := range tasks {
			response.Tasks = append(response.Tasks, TaskResponse{ID: value.ID, Key: value.Key, Status: value.Status,
				Input: value.Input.Value, Output: value.Output.Value, AttemptCount: value.AttemptCount,
				MaxAttemptCount: value.MaxAttemptCount, CurrentAttemptID: value.CurrentAttemptID})
		}
		writeJSON(w, 200, response)
	}
}
