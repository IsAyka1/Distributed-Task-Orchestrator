package tasks

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/task"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/workflow"
)

type completionFake struct {
	calls             []string
	fail              string
	failure           error
	now               time.Time
	snapshot          Snapshot
	closed, completed Completion
	wakeup            Wakeup
	revisionID        string
}

func newCompletionFake() *completionFake {
	now := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	expires, owner, token := now.Add(time.Minute), "worker", "attempt"
	return &completionFake{now: now, failure: errors.New("injected"), snapshot: Snapshot{
		TaskID: "task", WorkflowID: "workflow", WorkflowStatus: workflow.Running,
		AttemptID: token, AttemptWorkerID: owner, ActiveAttemptID: &token, LeaseOwner: &owner, LeaseExpiresAt: &expires,
		State: task.State{Status: task.Running, MaxAttemptCount: 1, AttemptCount: 1, LastAttempt: task.Attempt{Number: 1, Status: task.AttemptRunning}},
	}}
}
func completionRequest() CompletionRequest {
	return CompletionRequest{TaskID: "task", AttemptID: "attempt", WorkerID: "worker", Event: task.Succeed,
		Output: workflow.Payload{Value: []byte(`{"n":2}`)}}
}
func (f *completionFake) call(name string) error {
	f.calls = append(f.calls, name)
	if f.fail == name {
		return f.failure
	}
	return nil
}
func (f *completionFake) Begin(context.Context) (Transaction, error) { return f, f.call("begin") }
func (f *completionFake) LockAttempt(context.Context, CompletionRequest) (Snapshot, error) {
	return f.snapshot, f.call("lock")
}
func (f *completionFake) Now(context.Context) (time.Time, error) { return f.now, f.call("now") }
func (f *completionFake) CloseAttempt(_ context.Context, c Completion) error {
	f.closed = c
	return f.call("close")
}
func (f *completionFake) CompleteTask(_ context.Context, c Completion) error {
	f.completed = c
	return f.call("complete")
}
func (f *completionFake) Enqueue(_ context.Context, id string, at time.Time) (Wakeup, error) {
	f.wakeup = Wakeup{WorkflowID: id, CreatedAt: at}
	return f.wakeup, f.call("enqueue")
}
func (f *completionFake) IncrementVersion(_ context.Context, id string) error {
	f.revisionID = id
	return f.call("revision")
}
func (f *completionFake) Commit() error   { return f.call("commit") }
func (f *completionFake) Rollback() error { return f.call("rollback") }

func TestCompletionCommitsBothOutcomesAndWakeup(t *testing.T) {
	for _, event := range []task.Event{task.Succeed, task.Fail} {
		t.Run(string(event), func(t *testing.T) {
			f, request := newCompletionFake(), completionRequest()
			request.Event = event
			wantStatus, wantAttempt := task.Succeeded, task.AttemptSucceeded
			if event == task.Fail {
				request.Output.Value = nil
				request.ErrorType, request.ErrorMessage = "permanent", "activity failed"
				wantStatus, wantAttempt = task.Failed, task.AttemptFailed
			}
			if err := (Service{Repository: f}).CompleteAttempt(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			got := f.completed
			if !reflect.DeepEqual(got, f.closed) || got.TaskID != "task" || got.AttemptID != "attempt" ||
				got.State.Status != wantStatus || got.State.LastAttempt.Status != wantAttempt || !got.At.Equal(f.now) ||
				got.ErrorType != request.ErrorType || got.ErrorMessage != request.ErrorMessage || f.revisionID != "workflow" ||
				f.wakeup != (Wakeup{WorkflowID: "workflow", CreatedAt: f.now}) {
				t.Fatalf("completion: %+v", f)
			}
			wantOutput := `{"n":2}`
			if event == task.Fail {
				wantOutput = "null"
			}
			if string(got.Output.Value) != wantOutput {
				t.Fatal(string(got.Output.Value))
			}
			wantCalls := []string{"begin", "lock", "now", "close", "complete", "enqueue", "revision", "commit", "rollback"}
			if !reflect.DeepEqual(f.calls, wantCalls) {
				t.Fatal(f.calls)
			}
		})
	}
}

func TestCompletionRejectsStaleSnapshotWithoutWrites(t *testing.T) {
	for name, mutate := range map[string]func(*completionFake){
		"workflow pending":    func(f *completionFake) { f.snapshot.WorkflowStatus = workflow.Pending },
		"workflow terminal":   func(f *completionFake) { f.snapshot.WorkflowStatus = workflow.Succeeded },
		"task terminal":       func(f *completionFake) { f.snapshot.State.Status = task.Succeeded },
		"token absent":        func(f *completionFake) { f.snapshot.ActiveAttemptID = nil },
		"token wrong":         func(f *completionFake) { token := "other"; f.snapshot.ActiveAttemptID = &token },
		"owner absent":        func(f *completionFake) { f.snapshot.LeaseOwner = nil },
		"owner wrong":         func(f *completionFake) { owner := "other"; f.snapshot.LeaseOwner = &owner },
		"attempt owner wrong": func(f *completionFake) { f.snapshot.AttemptWorkerID = "other" },
		"attempt closed":      func(f *completionFake) { f.snapshot.State.LastAttempt.Status = task.AttemptFailed },
		"lease absent":        func(f *completionFake) { f.snapshot.LeaseExpiresAt = nil },
		"expiry equality":     func(f *completionFake) { f.now = *f.snapshot.LeaseExpiresAt },
		"expired":             func(f *completionFake) { f.now = f.snapshot.LeaseExpiresAt.Add(time.Microsecond) },
	} {
		t.Run(name, func(t *testing.T) {
			f := newCompletionFake()
			mutate(f)
			err := (Service{Repository: f}).CompleteAttempt(context.Background(), completionRequest())
			if !errors.Is(err, ErrStaleAttempt) {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(f.calls, []string{"begin", "lock", "now", "rollback"}) {
				t.Fatal(f.calls)
			}
		})
	}
}

func TestCompletionRejectsInvalidRequestsBeforeTransaction(t *testing.T) {
	for name, mutate := range map[string]func(*CompletionRequest){
		"empty owner":   func(r *CompletionRequest) { r.WorkerID = "" },
		"invalid owner": func(r *CompletionRequest) { r.WorkerID = string([]byte{255}) },
		"nul owner":     func(r *CompletionRequest) { r.WorkerID = "bad\x00worker" },
		"event":         func(r *CompletionRequest) { r.Event = task.Claim },
		"json":          func(r *CompletionRequest) { r.Output.Value = []byte("{") },
		"utf8":          func(r *CompletionRequest) { r.Output.Value = []byte{'"', 255, '"'} },
		"success error": func(r *CompletionRequest) { r.ErrorType = "error" },
		"failed output": func(r *CompletionRequest) { r.Event = task.Fail },
		"error nul":     func(r *CompletionRequest) { r.Event = task.Fail; r.Output.Value = nil; r.ErrorMessage = "x\x00y" },
		"error utf8": func(r *CompletionRequest) {
			r.Event = task.Fail
			r.Output.Value = nil
			r.ErrorType = string([]byte{255})
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := completionRequest()
			mutate(&r)
			if err := (Service{}).CompleteAttempt(context.Background(), r); !errors.Is(err, ErrInvalidRequest) {
				t.Fatal(err)
			}
		})
	}
}

func TestCompletionFailuresRollbackAndReturnError(t *testing.T) {
	for _, stage := range []string{"begin", "lock", "now", "close", "complete", "enqueue", "revision", "commit"} {
		t.Run(stage, func(t *testing.T) {
			f := newCompletionFake()
			f.fail = stage
			err := (Service{Repository: f}).CompleteAttempt(context.Background(), completionRequest())
			if !errors.Is(err, f.failure) {
				t.Fatal(err)
			}
			if stage != "begin" && f.calls[len(f.calls)-1] != "rollback" {
				t.Fatal(f.calls)
			}
		})
	}
}

func TestCompletionValidatesDomainAndDetachesOutput(t *testing.T) {
	f := newCompletionFake()
	f.snapshot.State.MaxAttemptCount = 2
	if err := (Service{Repository: f}).CompleteAttempt(context.Background(), completionRequest()); !errors.Is(err, task.ErrInvalidTransition) {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.calls, []string{"begin", "lock", "now", "rollback"}) {
		t.Fatal(f.calls)
	}
	f = newCompletionFake()
	request := completionRequest()
	if err := (Service{Repository: f}).CompleteAttempt(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	request.Output.Value[0] = '['
	if string(f.completed.Output.Value) != `{"n":2}` {
		t.Fatal("retained caller payload")
	}
	f = newCompletionFake()
	request.Output.Value = nil
	if err := (Service{Repository: f}).CompleteAttempt(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if string(f.completed.Output.Value) != "null" {
		t.Fatal("missing output must mean successful null")
	}
}
