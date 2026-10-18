package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/queue"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/tasks"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/task"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/workflow"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/tests/support/clock"
)

func newClock() *clock.Manual {
	return clock.New(time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC))
}

type activityFunc func(context.Context, workflow.Payload, string) (workflow.Payload, error)

func (f activityFunc) Execute(ctx context.Context, p workflow.Payload, key string) (workflow.Payload, error) {
	return f(ctx, p, key)
}

type fixture struct {
	t                                           *testing.T
	clock                                       *clock.Manual
	runner                                      Runner
	claimed                                     queue.ClaimedTask
	claimErr, nextErr, evaluateErr, completeErr error
	wakeup                                      string
	claims, evaluations                         int
	reports                                     []tasks.CompletionRequest
	execute                                     activityFunc
	onClaim                                     func(context.Context)
	onComplete                                  func(context.Context)
}

func setup(t *testing.T) *fixture {
	f := &fixture{t: t, clock: newClock(), claimed: queue.ClaimedTask{TaskID: "stable-task", Key: "A", Attempt: queue.Attempt{ID: "attempt", WorkerID: "worker"}}}
	f.execute = func(context.Context, workflow.Payload, string) (workflow.Payload, error) {
		return workflow.Payload{Value: []byte(`{"n":2}`)}, nil
	}
	registry, err := NewRegistry(Registration{Key: "A", Activity: activityFunc(func(ctx context.Context, p workflow.Payload, key string) (workflow.Payload, error) {
		return f.execute(ctx, p, key)
	})})
	if err != nil {
		t.Fatal(err)
	}
	f.runner = Runner{Queue: f, Completer: f, Evaluator: f, Repository: f, Registry: registry, Clock: f.clock, Config: Config{WorkerID: "worker", PollInterval: time.Second, OperationTimeout: time.Second, LeaseDuration: 10 * time.Second}}
	return f
}
func (f *fixture) Claim(ctx context.Context, r queue.ClaimRequest) (queue.ClaimedTask, error) {
	f.claims++
	if r.WorkerID != "worker" || r.LeaseDuration != 10*time.Second {
		f.t.Errorf("claim: %+v", r)
	}
	if f.onClaim != nil {
		f.onClaim(ctx)
	}
	return f.claimed, f.claimErr
}
func (f *fixture) NextWakeup(context.Context) (string, error) { return f.wakeup, f.nextErr }
func (f *fixture) EvaluateWorkflow(_ context.Context, id string) error {
	if id != f.wakeup {
		f.t.Errorf("workflow: %s", id)
	}
	f.evaluations++
	return f.evaluateErr
}
func (f *fixture) CompleteAttempt(ctx context.Context, r tasks.CompletionRequest) error {
	f.reports = append(f.reports, r)
	if f.onComplete != nil {
		f.onComplete(ctx)
	}
	return f.completeErr
}
func TestRegistry(t *testing.T) {
	a := activityFunc(func(context.Context, workflow.Payload, string) (workflow.Payload, error) {
		return workflow.Payload{}, nil
	})
	for _, entries := range [][]Registration{{{Key: "", Activity: a}}, {{Key: "A"}}, {{Key: "A", Activity: a}, {Key: "A", Activity: a}}} {
		if _, err := NewRegistry(entries...); err == nil {
			t.Fatal("invalid registry accepted")
		}
	}
	entries := []Registration{{Key: "A", Activity: a}}
	r, err := NewRegistry(entries...)
	if err != nil {
		t.Fatal(err)
	}
	entries[0].Activity = nil
	if r.activities["A"] == nil {
		t.Fatal("caller-owned registry")
	}
}
func TestExecutionUsesStableTaskID(t *testing.T) {
	f := setup(t)
	f.wakeup = "workflow"
	f.claimed.Input = workflow.Payload{Value: []byte(`{"n":1}`)}
	f.execute = func(_ context.Context, p workflow.Payload, key string) (workflow.Payload, error) {
		if key != "stable-task" || string(p.Value) != `{"n":1}` || f.evaluations != 1 {
			t.Fatalf("input/key/order: %s %s %d", p.Value, key, f.evaluations)
		}
		return workflow.Payload{Value: []byte(`{"n":2}`)}, nil
	}
	worked, err := f.runner.Step(context.Background())
	if err != nil || !worked || len(f.reports) != 1 {
		t.Fatalf("step: %v %v %+v", worked, err, f.reports)
	}
	r := f.reports[0]
	if r.TaskID != "stable-task" || r.AttemptID != "attempt" || r.WorkerID != "worker" || r.Event != task.Succeed || string(r.Output.Value) != `{"n":2}` {
		t.Fatalf("report: %+v", r)
	}
}
func TestActivityFailures(t *testing.T) {
	for _, name := range []string{"unknown", "error", "timeout", "claim_elapsed"} {
		t.Run(name, func(t *testing.T) {
			f := setup(t)
			expected := "activity_failed"
			calls := 0
			f.execute = func(ctx context.Context, _ workflow.Payload, _ string) (workflow.Payload, error) {
				calls++
				if name == "timeout" {
					f.clock.Advance(9 * time.Second)
					<-ctx.Done()
					return workflow.Payload{}, ctx.Err()
				}
				return workflow.Payload{Value: []byte(`"secret"`)}, errors.New("secret failure")
			}
			switch name {
			case "unknown":
				f.claimed.Key = "missing"
				expected = "unknown_activity"
			case "timeout":
				expected = "activity_timeout"
				f.onComplete = func(ctx context.Context) {
					if ctx.Err() != nil {
						t.Fatal("completion budget already expired")
					}
					f.clock.Advance(time.Second)
					if ctx.Err() == nil {
						t.Fatal("completion is unbounded")
					}
				}
			case "claim_elapsed":
				f.onClaim = func(context.Context) { f.clock.Advance(9 * time.Second) }
				expected = "activity_timeout"
			}
			_, err := f.runner.Step(context.Background())
			if err != nil || len(f.reports) != 1 {
				t.Fatalf("step: %v %+v", err, f.reports)
			}
			r := f.reports[0]
			if r.Event != task.Fail || r.ErrorType != expected || r.ErrorMessage != "" || len(r.Output.Value) != 0 {
				t.Fatalf("report: %+v", r)
			}
			if (name == "unknown" || name == "claim_elapsed") && calls != 0 {
				t.Fatal("ineligible activity executed")
			}
		})
	}
}
func TestNoExecutionAfterClaimError(t *testing.T) {
	for _, failure := range []error{queue.ErrNoTask, errors.New("uncertain commit")} {
		t.Run(failure.Error(), func(t *testing.T) {
			f := setup(t)
			f.claimErr = failure
			f.execute = func(context.Context, workflow.Payload, string) (workflow.Payload, error) {
				t.Fatal("executed provisional claim")
				return workflow.Payload{}, nil
			}
			worked, err := f.runner.Step(context.Background())
			if worked || len(f.reports) != 0 || (failure == queue.ErrNoTask && err != nil) || (failure != queue.ErrNoTask && !errors.Is(err, failure)) {
				t.Fatalf("step: %v %v", worked, err)
			}
		})
	}
}
func TestLoopPollsAndStopsWithControlledClock(t *testing.T) {
	f := setup(t)
	f.claimErr = queue.ErrNoTask
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- f.runner.Run(ctx) }()
	<-f.clock.Waiting
	f.clock.Advance(time.Second)
	<-f.clock.Waiting
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if f.claims != 2 {
		t.Fatalf("polls: %d", f.claims)
	}
}
func TestShutdownDuringActivity(t *testing.T) {
	f := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.execute = func(ctx context.Context, _ workflow.Payload, _ string) (workflow.Payload, error) {
		cancel()
		<-ctx.Done()
		return workflow.Payload{}, nil
	}
	if err := f.runner.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if len(f.reports) != 0 {
		t.Fatal("completion after shutdown")
	}
}
func TestLoopStopsOnFailure(t *testing.T) {
	for _, where := range []string{"discovery", "evaluation", "completion"} {
		t.Run(where, func(t *testing.T) {
			f := setup(t)
			failure := errors.New("database unavailable")
			switch where {
			case "discovery":
				f.nextErr = failure
			case "evaluation":
				f.wakeup = "workflow"
				f.evaluateErr = failure
			case "completion":
				f.completeErr = failure
			}
			if err := f.runner.Run(context.Background()); !errors.Is(err, failure) {
				t.Fatalf("error: %v", err)
			}
			if where != "completion" && f.claims != 0 {
				t.Fatal("claim after evaluation error")
			}
			if len(f.reports) > 1 {
				t.Fatal("retried uncertain completion")
			}
		})
	}
}

func TestInvalidConfigurationDoesNotClaim(t *testing.T) {
	for _, invalid := range []Config{
		{WorkerID: "", PollInterval: time.Second, OperationTimeout: time.Second, LeaseDuration: time.Minute},
		{WorkerID: "bad\x00", PollInterval: time.Second, OperationTimeout: time.Second, LeaseDuration: time.Minute},
		{WorkerID: "bad\xff", PollInterval: time.Second, OperationTimeout: time.Second, LeaseDuration: time.Minute},
		{WorkerID: "worker", OperationTimeout: time.Second, LeaseDuration: time.Minute},
		{WorkerID: "worker", PollInterval: time.Second, LeaseDuration: time.Minute},
		{WorkerID: "worker", PollInterval: time.Second, OperationTimeout: time.Second, LeaseDuration: time.Second},
		{WorkerID: "worker", PollInterval: time.Second, OperationTimeout: time.Second, LeaseDuration: time.Minute + 1},
	} {
		f := setup(t)
		f.runner.Config = invalid
		if err := f.runner.Run(context.Background()); err == nil || f.claims != 0 {
			t.Fatalf("invalid config: %v, claims %d", err, f.claims)
		}
	}
}
