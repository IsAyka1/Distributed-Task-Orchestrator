package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/definitions"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/workflows"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/workflow"
	testclock "github.com/IsAyka1/Distributed-Task-Orchestrator/tests/support/clock"
	"github.com/jackc/pgx/v5/pgconn"
)

const validDefinition = `{"provider":" demo ","name":"sequence","version":1,"tasks":[{"id":"A","type":"activity","depends_on":[]}]}`
const validStart = `{"definition_id":"11111111-1111-4111-8111-111111111111","input":{"n":1}}`

type servicesFake struct {
	err          error
	publications []workflow.DefinitionSpec
	starts       []workflows.StartRequest
	entered      chan struct{}
}

func (s *servicesFake) Publish(ctx context.Context, spec workflow.DefinitionSpec) (definitions.PublishedDefinition, error) {
	s.publications = append(s.publications, spec)
	if s.err != nil {
		return definitions.PublishedDefinition{}, s.err
	}
	d, err := definitions.ValidatePublication(spec)
	return definitions.PublishedDefinition{ID: "published-id", Definition: d}, err
}
func (s *servicesFake) StartWorkflow(ctx context.Context, request workflows.StartRequest) (workflows.StartedWorkflow, error) {
	s.starts = append(s.starts, request)
	if s.entered != nil {
		close(s.entered)
		<-ctx.Done()
		if s.err != nil {
			return workflows.StartedWorkflow{}, s.err
		}
		return workflows.StartedWorkflow{}, ctx.Err()
	}
	return workflows.StartedWorkflow{ID: "run-id", Status: workflow.Pending}, s.err
}

type fixture struct {
	services *servicesFake
	clock    *testclock.Manual
	handler  Handler
}

func setup() *fixture {
	s := &servicesFake{}
	c := testclock.New(time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC))
	return &fixture{s, c, Handler{Definitions: s, Workflows: s, Clock: c, Timeout: time.Second}}
}
func (f *fixture) request(path, body, media string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", media)
	req.ContentLength = -1 // Exercise the streaming limit, independently of Content-Length.
	result := httptest.NewRecorder()
	f.handler.ServeHTTP(result, req)
	return result
}
func assertError(t *testing.T, r *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	var value ErrorResponse
	if err := json.Unmarshal(r.Body.Bytes(), &value); err != nil {
		t.Fatal(err, r.Body.String())
	}
	if r.Code != status || value.Error.Code != code || value.Error.Message == "" {
		t.Fatalf("response: %d %s", r.Code, r.Body.String())
	}
	if strings.Contains(r.Body.String(), "secret") {
		t.Fatal("internal error leaked")
	}
	if r.Header().Get("Content-Type") != "application/json" {
		t.Fatal("JSON content type missing")
	}
}
func TestCreationResponses(t *testing.T) {
	f := setup()
	r := f.request("/workflow-definitions", validDefinition, "application/json; charset=utf-8")
	var published struct {
		ID, Provider, Name string
		Version            int64
	}
	if err := json.Unmarshal(r.Body.Bytes(), &published); err != nil {
		t.Fatal(err)
	}
	if r.Code != 201 || published.ID != "published-id" || published.Provider != " demo " || published.Version != 1 {
		t.Fatal(r.Code, r.Body.String())
	}
	if len(f.services.publications) != 1 || f.services.publications[0].Tasks[0].MaxAttempts != nil {
		t.Fatal("publication changed")
	}
	r = f.request("/workflows", validStart, "application/json")
	if r.Code != 201 || r.Body.String() != "{\"id\":\"run-id\",\"status\":\"PENDING\"}\n" {
		t.Fatal(r.Code, r.Body.String())
	}
	if string(f.services.starts[0].Input.Value) != `{"n":1}` {
		t.Fatal("payload changed")
	}
}
func TestInvalidBodiesDoNotCallServices(t *testing.T) {
	cases := []struct{ path, body string }{
		{"/workflows", ``}, {"/workflows", `null`}, {"/workflows", `[]`}, {"/workflows", `{}`},
		{"/workflows", `{"definition_id":null}`}, {"/workflows", `{"definition_id":12}`},
		{"/workflows", validStart + ` {}`}, {"/workflows", `{"definition_id":"id","extra":1}`},
		{"/workflows", `{"definition_id":"id","input":"` + string([]byte{0xff}) + `"}`},
		{"/workflows", `{"definition_id":"id","input":"\ud800"}`},
		{"/workflow-definitions", `{"provider":"demo","name":"flow","tasks":[]}`},
		{"/workflow-definitions", strings.Replace(validDefinition, `"version":1`, `"version":1.5`, 1)},
		{"/workflow-definitions", strings.Replace(validDefinition, `"type":"activity"`, `"type":null`, 1)},
		{"/workflow-definitions", strings.Replace(validDefinition, `"depends_on":[]`, `"depends_on":[null]`, 1)},
		{"/workflow-definitions", strings.Replace(validDefinition, `"depends_on":[]`, `"depends_on":[],"extra":1`, 1)},
		{"/workflow-definitions", strings.Replace(validDefinition, `"depends_on":[]`, `"depends_on":[],"max_attempts":null`, 1)},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			f := setup()
			assertError(t, f.request(tc.path, tc.body, "application/json"), 400, "invalid_request")
			if len(f.services.starts)+len(f.services.publications) != 0 {
				t.Fatal("invalid body reached service")
			}
		})
	}
}
func TestBodyLimitAndMedia(t *testing.T) {
	for _, path := range []string{"/workflows", "/workflow-definitions"} {
		f := setup()
		assertError(t, f.request(path, strings.Repeat(" ", (1<<20)+1), "application/json"), 413, "request_too_large")
		for _, media := range []string{"", "text/plain", "application/json; charset=latin1", "application/json; broken"} {
			assertError(t, f.request(path, validStart, media), 415, "unsupported_media_type")
		}
		if len(f.services.starts)+len(f.services.publications) != 0 {
			t.Fatal("rejected body reached services")
		}
	}
	f := setup()
	atLimit := validStart + strings.Repeat(" ", (1<<20)-len(validStart))
	if r := f.request("/workflows", atLimit, "application/json"); r.Code != 201 {
		t.Fatal(r.Code, r.Body.String())
	}
}
func TestErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{definitions.ErrDefinitionExists, 409, "definition_version_conflict"},
		{definitions.ErrDefinitionNotFound, 404, "definition_not_found"},
		{workflow.ErrInvalidDefinition, 422, "invalid_definition"},
		{workflow.ErrUnsupportedWorkflow, 422, "unsupported_workflow"},
		{workflows.ErrUnsupportedPolicy, 422, "unsupported_policy"},
		{workflows.ErrInvalidRequest, 400, "invalid_request"},
		{context.DeadlineExceeded, 503, "storage_unavailable"},
		{sql.ErrTxDone, 500, "internal_error"},
		{io.EOF, 503, "storage_unavailable"},
		{io.ErrUnexpectedEOF, 503, "storage_unavailable"},
		{pgconn.ErrConnClosed, 503, "storage_unavailable"},
		{&pgconn.PgError{Code: "57P01", Message: "secret"}, 503, "storage_unavailable"},
		{errors.New("secret unexpected failure"), 500, "internal_error"},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			f := setup()
			f.services.err = fmt.Errorf("secret context: %w", tc.err)
			assertError(t, f.request("/workflows", validStart, "application/json"), tc.status, tc.code)
		})
	}
}
func TestRequestTimeoutUsesInjectedClock(t *testing.T) {
	f := setup()
	f.services.entered = make(chan struct{})
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- f.request("/workflows", validStart, "application/json") }()
	<-f.services.entered
	f.clock.Advance(time.Second)
	assertError(t, <-done, 503, "storage_unavailable")
}

func TestUnicodeAndOpaquePayloads(t *testing.T) {
	for _, payload := range []string{`"\ud83d\ude00"`, `"\\ud800"`, `"é"`, `[1,true,null,{"extra":2}]`} {
		f := setup()
		r := f.request("/workflows", `{"definition_id":"id","input":`+payload+`}`, "application/json")
		if r.Code != 201 || string(f.services.starts[0].Input.Value) != payload {
			t.Fatal(r.Code, r.Body.String())
		}
	}
	for _, payload := range []string{`"\udc00"`, `"\ud800\u0041"`, `"\ud800x"`} {
		f := setup()
		assertError(t, f.request("/workflows", `{"definition_id":"id","input":`+payload+`}`, "application/json"), 400, "invalid_request")
	}
}
func TestRequiredDependencyArray(t *testing.T) {
	for _, field := range []string{`"depends_on":null`, `"max_attempts":1`} {
		f := setup()
		assertError(t, f.request("/workflow-definitions", strings.Replace(validDefinition, `"depends_on":[]`, field, 1), "application/json"), 400, "invalid_request")
	}
}
func TestUnsupportedRouteAndMethod(t *testing.T) {
	f := setup()
	assertError(t, f.request("/unknown", `{}`, "application/json"), 404, "not_found")
	req := httptest.NewRequest(http.MethodGet, "/workflows", nil)
	r := httptest.NewRecorder()
	f.handler.ServeHTTP(r, req)
	assertError(t, r, 405, "method_not_allowed")
	if r.Header().Get("Allow") != "POST" {
		t.Fatal("missing allowed method")
	}
}

func TestNonJSONWhitespaceRejected(t *testing.T) {
	for _, padding := range []string{"\v", "\f", "\u00a0", "\ufeff"} {
		for _, body := range []string{padding + validStart, validStart + padding} {
			f := setup()
			assertError(t, f.request("/workflows", body, "application/json"), 400, "invalid_request")
			if len(f.services.starts) != 0 {
				t.Fatal("non-JSON whitespace reached service")
			}
		}
	}
}

func TestErrorAfterRequestCancellation(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		for _, tc := range []struct {
			err    error
			status int
			code   string
		}{
			{sql.ErrTxDone, 503, "storage_unavailable"},
			{errors.New("secret unexpected failure"), 500, "internal_error"},
		} {
			t.Run(fmt.Sprintf("shutdown=%t/%s", shutdown, tc.code), func(t *testing.T) {
				f := setup()
				f.services.entered = make(chan struct{})
				f.services.err = fmt.Errorf("secret commit workflow: %w", tc.err)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				req := httptest.NewRequest(http.MethodPost, "/workflows", strings.NewReader(validStart)).WithContext(ctx)
				req.Header.Set("Content-Type", "application/json")
				result := httptest.NewRecorder()
				done := make(chan struct{})
				go func() {
					f.handler.ServeHTTP(result, req)
					close(done)
				}()
				<-f.services.entered
				if shutdown {
					cancel()
				} else {
					f.clock.Advance(time.Second)
				}
				<-done
				assertError(t, result, tc.status, tc.code)
				if len(f.services.starts) != 1 {
					t.Fatal("cancelled start was retried")
				}
			})
		}
	}
}
