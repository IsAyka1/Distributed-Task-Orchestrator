package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/definitions"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/workflows"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/workflow"
)

type Publisher interface {
	Publish(context.Context, workflow.DefinitionSpec) (definitions.PublishedDefinition, error)
}
type Starter interface {
	StartWorkflow(context.Context, workflows.StartRequest) (workflows.StartedWorkflow, error)
}
type Clock interface {
	WithTimeout(context.Context, time.Duration) (context.Context, context.CancelFunc)
}
type Handler struct {
	Definitions Publisher
	Workflows   Starter
	Inspection  Inspector
	Clock       Clock
	Timeout     time.Duration
}
type DefinitionResponse struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	Name     string `json:"name"`
	Version  int64  `json:"version"`
}
type StartResponse struct {
	ID     string          `json:"id"`
	Status workflow.Status `json:"status"`
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/workflows/") {
		h.serveInspection(w, r)
		return
	}
	if r.URL.Path != "/workflow-definitions" && r.URL.Path != "/workflows" {
		writeError(w, 404, "not_found", "This endpoint does not exist.")
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, 405, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	ctx, cancel := h.Clock.WithTimeout(r.Context(), h.Timeout)
	defer cancel()
	if r.URL.Path == "/workflow-definitions" {
		var request definitionRequest
		if !decode(w, r, &request) {
			return
		}
		spec, ok := request.spec()
		if !ok {
			writeServiceError(ctx, w, workflows.ErrInvalidRequest)
			return
		}
		published, err := h.Definitions.Publish(ctx, spec)
		if err != nil {
			writeServiceError(ctx, w, err)
			return
		}
		d := published.Definition
		writeJSON(w, 201, DefinitionResponse{published.ID, d.Provider(), d.Name(), d.Version()})
	} else {
		var request startRequest
		if !decode(w, r, &request) {
			return
		}
		if request.DefinitionID == nil {
			writeServiceError(ctx, w, workflows.ErrInvalidRequest)
			return
		}
		result, err := h.Workflows.StartWorkflow(ctx, workflows.StartRequest{DefinitionID: *request.DefinitionID, Input: workflow.Payload{Value: request.Input}})
		if err != nil {
			writeServiceError(ctx, w, err)
			return
		}
		writeJSON(w, 201, StartResponse{result.ID, result.Status})
	}
}
