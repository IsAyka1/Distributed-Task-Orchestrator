package queue

import (
	"context"
	"errors"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/task"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/workflow"
	"reflect"
	"testing"
	"time"
)

func TestClaimRejectsInvalidRequestBeforeOpeningTransaction(t *testing.T) {
	for _, request := range []ClaimRequest{
		{WorkerID: "", LeaseDuration: time.Minute},
		{WorkerID: "bad\x00worker", LeaseDuration: time.Minute},
		{WorkerID: string([]byte{0xff}), LeaseDuration: time.Minute},
		{WorkerID: "worker", LeaseDuration: 0},
		{WorkerID: "worker", LeaseDuration: -time.Second},
		{WorkerID: "worker", LeaseDuration: time.Nanosecond},
	} {
		_, err := (Service{}).Claim(context.Background(), request)
		if !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("invalid request accepted: %v", err)
		}
	}
}

type claimFake struct {
	calls     []string
	fail      string
	failure   error
	candidate Candidate
	now       time.Time
	updated   ClaimedTask
}

func newClaimFake() *claimFake {
	now := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	return &claimFake{now: now, failure: errors.New("injected"), candidate: Candidate{
		ID: "task", WorkflowID: "workflow", Key: "A", Input: workflow.Payload{Value: []byte(`{"n":1}`)},
		State: task.State{Status: task.Ready, MaxAttemptCount: 1}, AvailableAt: now,
	}}
}
func (f *claimFake) call(name string) error {
	f.calls = append(f.calls, name)
	if f.fail == name {
		return f.failure
	}
	return nil
}
func (f *claimFake) Begin(context.Context) (Transaction, error)  { return f, f.call("begin") }
func (f *claimFake) LockNext(context.Context) (Candidate, error) { return f.candidate, f.call("lock") }
func (f *claimFake) Now(context.Context) (time.Time, error)      { return f.now, f.call("now") }
func (f *claimFake) InsertAttempt(_ context.Context, value AttemptStart) (Attempt, error) {
	return Attempt{ID: "attempt", TaskID: value.TaskID, Number: value.Number, WorkerID: value.WorkerID,
		StartedAt: value.At, HeartbeatAt: value.At, Status: task.AttemptRunning}, f.call("insert")
}
func (f *claimFake) UpdateTask(_ context.Context, value ClaimedTask) error {
	f.updated = value
	return f.call("update")
}
func (f *claimFake) Enqueue(_ context.Context, id string, at time.Time) (Wakeup, error) {
	return Wakeup{WorkflowID: id, CreatedAt: at}, f.call("enqueue")
}
func (f *claimFake) IncrementVersion(context.Context, string) error { return f.call("revision") }
func (f *claimFake) Commit() error                                  { return f.call("commit") }
func (f *claimFake) Rollback() error                                { return f.call("rollback") }

func TestClaimCommitsAttemptLeaseAndWakeup(t *testing.T) {
	f := newClaimFake()
	request := ClaimRequest{WorkerID: " worker ", LeaseDuration: time.Minute}
	got, err := (Service{Repository: f}).Claim(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if got.TaskID != "task" || got.WorkflowID != "workflow" || got.Key != "A" ||
		got.Attempt.ID != "attempt" || got.Attempt.Number != 1 || got.Attempt.WorkerID != request.WorkerID ||
		got.Attempt.Status != task.AttemptRunning || !got.Attempt.StartedAt.Equal(f.now) ||
		!got.LeaseExpiresAt.Equal(f.now.Add(time.Minute)) || string(got.Input.Value) != `{"n":1}` {
		t.Fatalf("claim: %+v", got)
	}
	if !reflect.DeepEqual(f.updated, got) {
		t.Fatalf("persisted claim differs: %+v", f.updated)
	}
	want := []string{"begin", "lock", "now", "insert", "update", "enqueue", "revision", "commit", "rollback"}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("calls: %v", f.calls)
	}
}
func TestClaimRechecksAvailabilityAndTransition(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		change func(*claimFake)
		want   error
	}{
		{"future", func(f *claimFake) { f.candidate.AvailableAt = f.now.Add(time.Microsecond) }, ErrNoTask},
		{"pending", func(f *claimFake) { f.candidate.State.Status = task.Pending }, task.ErrInvalidTransition},
		{"exhausted", func(f *claimFake) { f.candidate.State.AttemptCount = 1 }, task.ErrInvalidTransition},
		{"unsupported", func(f *claimFake) { f.candidate.State.MaxAttemptCount = 2 }, task.ErrInvalidTransition},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := newClaimFake()
			scenario.change(f)
			got, err := (Service{Repository: f}).Claim(context.Background(), ClaimRequest{"worker", time.Minute})
			if !errors.Is(err, scenario.want) || !reflect.DeepEqual(got, ClaimedTask{}) {
				t.Fatalf("claim=%+v err=%v", got, err)
			}
			for _, call := range f.calls {
				if call == "insert" || call == "commit" {
					t.Fatalf("unexpected %s", call)
				}
			}
			if f.calls[len(f.calls)-1] != "rollback" {
				t.Fatal(f.calls)
			}
		})
	}
}
func TestClaimFailuresReturnNoWorkAndRollback(t *testing.T) {
	for _, stage := range []string{"begin", "lock", "now", "insert", "update", "enqueue", "revision", "commit"} {
		t.Run(stage, func(t *testing.T) {
			f := newClaimFake()
			f.fail = stage
			got, err := (Service{Repository: f}).Claim(context.Background(), ClaimRequest{"worker", time.Minute})
			if !errors.Is(err, f.failure) || !reflect.DeepEqual(got, ClaimedTask{}) {
				t.Fatalf("claim=%+v err=%v", got, err)
			}
			if stage != "begin" && f.calls[len(f.calls)-1] != "rollback" {
				t.Fatal(f.calls)
			}
		})
	}
	f := newClaimFake()
	f.fail = "lock"
	f.failure = ErrNoTask
	if _, err := (Service{Repository: f}).Claim(context.Background(), ClaimRequest{"worker", time.Minute}); !errors.Is(err, ErrNoTask) {
		t.Fatal(err)
	}
}
