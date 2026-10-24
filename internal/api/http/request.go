package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/workflow"
)

const maxBodyBytes = 1 << 20

type definitionRequest struct {
	Provider *string       `json:"provider"`
	Name     *string       `json:"name"`
	Version  *int64        `json:"version"`
	Tasks    []taskRequest `json:"tasks"`
}
type taskRequest struct {
	ID          *string          `json:"id"`
	Type        *string          `json:"type"`
	DependsOn   []*string        `json:"depends_on"`
	MaxAttempts optionalAttempts `json:"max_attempts"`
}
type optionalAttempts struct{ value *int64 }

func (a *optionalAttempts) UnmarshalJSON(raw []byte) error {
	if bytes.Equal(raw, []byte("null")) {
		return errors.New("integer required")
	}
	var value int64
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	a.value = &value
	return nil
}

type startRequest struct {
	DefinitionID *string         `json:"definition_id"`
	Input        json.RawMessage `json:"input"`
}

func (r definitionRequest) spec() (workflow.DefinitionSpec, bool) {
	if r.Provider == nil || r.Name == nil || r.Version == nil || r.Tasks == nil {
		return workflow.DefinitionSpec{}, false
	}
	spec := workflow.DefinitionSpec{Provider: *r.Provider, Name: *r.Name, Version: *r.Version}
	for _, t := range r.Tasks {
		if t.ID == nil || t.Type == nil || t.DependsOn == nil {
			return workflow.DefinitionSpec{}, false
		}
		task := workflow.TaskSpec{ID: *t.ID, Type: *t.Type, MaxAttempts: t.MaxAttempts.value}
		for _, dependency := range t.DependsOn {
			if dependency == nil {
				return workflow.DefinitionSpec{}, false
			}
			task.DependsOn = append(task.DependsOn, *dependency)
		}
		spec.Tasks = append(spec.Tasks, task)
	}
	return spec, true
}
func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || (params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8")) {
		writeError(w, 415, "unsupported_media_type", "Use application/json with UTF-8 encoding.")
		return false
	}
	body := http.MaxBytesReader(w, r.Body, maxBodyBytes)
	defer body.Close()
	raw, err := io.ReadAll(body)
	var limit *http.MaxBytesError
	if errors.As(err, &limit) {
		writeError(w, 413, "request_too_large", "The request exceeds 1 MiB.")
		return false
	}
	raw = bytes.Trim(raw, " \t\r\n")
	if err != nil || len(raw) == 0 || raw[0] != '{' || !json.Valid(raw) || !utf8.Valid(raw) || !validEscapes(raw) {
		writeError(w, 400, "invalid_request", "Provide one valid UTF-8 JSON object.")
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, 400, "invalid_request", "Check the request fields and their types.")
		return false
	}
	return true
}

// encoding/json replaces unpaired UTF-16 surrogates; reject them before identity decoding.
// JSON syntax has already been checked, so each Unicode escape has four hex digits.
func validEscapes(raw []byte) bool {
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\\' {
			continue
		}
		i++
		if raw[i] != 'u' {
			continue
		}
		value, _ := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
		i += 4
		if value < 0xd800 || value > 0xdfff {
			continue
		}
		if value > 0xdbff || i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
			return false
		}
		low, _ := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
		if low < 0xdc00 || low > 0xdfff {
			return false
		}
		i += 6
	}
	return true
}
