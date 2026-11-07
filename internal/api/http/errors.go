package httpapi

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/definitions"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/workflows"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/workflow"
	"github.com/jackc/pgx/v5/pgconn"
)

type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// A failed response write cannot roll back a committed POST; never retry the use case.
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, ErrorResponse{ErrorDetail{code, message}})
}
func writeServiceError(ctx context.Context, w http.ResponseWriter, err error) {
	// database/sql can finish cancellation's rollback before the service calls Commit.
	if errors.Is(err, sql.ErrTxDone) && ctx.Err() != nil {
		err = ctx.Err()
	}
	switch {
	case errors.Is(err, workflows.ErrInvalidRequest), errors.Is(err, definitions.ErrInvalidDefinitionID):
		writeError(w, 400, "invalid_request", "Check the request fields and input.")
	case errors.Is(err, workflow.ErrInvalidDefinition):
		writeError(w, 422, "invalid_definition", "The definition is invalid.")
	case errors.Is(err, workflow.ErrUnsupportedWorkflow):
		writeError(w, 422, "unsupported_workflow", "Only sequential workflows can start.")
	case errors.Is(err, workflows.ErrUnsupportedPolicy):
		writeError(w, 422, "unsupported_policy", "Only one attempt per task is supported.")
	case errors.Is(err, workflows.ErrWorkflowNotFound):
		writeError(w, 404, "workflow_not_found", "The workflow does not exist.")
	case errors.Is(err, definitions.ErrDefinitionNotFound):
		writeError(w, 404, "definition_not_found", "The definition does not exist.")
	case errors.Is(err, definitions.ErrDefinitionExists):
		writeError(w, 409, "definition_version_conflict", "This definition version already exists.")
	case storageUnavailable(err):
		writeError(w, 503, "storage_unavailable", "Storage is unavailable; a write may have committed. GET requests may be retried. Do not blindly retry a POST.")
	default:
		writeError(w, 500, "internal_error", "The request could not be completed.")
	}
}
func storageUnavailable(err error) bool {
	var connection *pgconn.ConnectError
	if errors.As(err, &connection) {
		return true
	}
	var network net.Error
	var postgres *pgconn.PgError
	if errors.As(err, &network) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, driver.ErrBadConn) || errors.Is(err, sql.ErrConnDone) {
		return true
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, pgconn.ErrConnClosed) {
		return true
	}
	if errors.As(err, &postgres) {
		return strings.HasPrefix(postgres.Code, "08") || strings.HasPrefix(postgres.Code, "53") || strings.HasPrefix(postgres.Code, "57P") || postgres.Code == "57014" || postgres.Code == "40001" || postgres.Code == "40P01"
	}
	return false
}
