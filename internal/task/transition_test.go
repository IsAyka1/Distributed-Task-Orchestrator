package task_test

import (
	"errors"
	"math"
	"testing"

	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/task"
)

func states() []task.State {
	return []task.State{
		{Status: task.Pending, MaxAttemptCount: 1},
		{Status: task.Ready, MaxAttemptCount: 1},
		{Status: task.Running, MaxAttemptCount: 1, AttemptCount: 1, LastAttempt: task.Attempt{Number: 1, Status: task.AttemptRunning}},
		{Status: task.Succeeded, MaxAttemptCount: 1, AttemptCount: 1, LastAttempt: task.Attempt{Number: 1, Status: task.AttemptSucceeded}},
		{Status: task.Failed, MaxAttemptCount: 1, AttemptCount: 1, LastAttempt: task.Attempt{Number: 1, Status: task.AttemptFailed}},
	}
}

func TestNewState(t *testing.T) {
	got, err := task.NewState(1)
	if err != nil || got != states()[0] {
		t.Fatalf("NewState(1) = %+v, %v", got, err)
	}
	for _, limit := range []int32{-1, 0, 2, math.MaxInt32} {
		got, err := task.NewState(limit)
		if !errors.Is(err, task.ErrUnsupportedPolicy) || got != (task.State{}) {
			t.Fatalf("NewState(%d) = %+v, %v", limit, got, err)
		}
	}
}

func TestTransitionMatrix(t *testing.T) {
	type allowedTransition struct {
		from  task.Status
		event task.Event
		to    task.State
	}
	all := states()
	allowed := []allowedTransition{
		{task.Pending, task.Activate, all[1]},
		{task.Ready, task.Claim, all[2]},
		{task.Running, task.Succeed, all[3]},
		{task.Running, task.Fail, all[4]},
	}
	for _, before := range all {
		for _, event := range []task.Event{task.Activate, task.Claim, task.Succeed, task.Fail, "", "RETRY", "TIMEOUT"} {
			t.Run(string(before.Status)+"/"+string(event), func(t *testing.T) {
				input := before
				got, err := task.Transition(input, event)
				if input != before {
					t.Fatal("input mutated")
				}
				for _, transition := range allowed {
					if transition.from == before.Status && transition.event == event {
						if err != nil || got != transition.to {
							t.Fatalf("got %+v, %v; want %+v", got, err, transition.to)
						}
						return
					}
				}
				if !errors.Is(err, task.ErrInvalidTransition) || got != before {
					t.Fatalf("rejected transition = %+v, %v; want unchanged %+v", got, err, before)
				}
			})
		}
	}
}

func TestInvalidSnapshots(t *testing.T) {
	type invalidCase struct {
		name   string
		change func(*task.State)
	}
	cases := []invalidCase{
		{"zero limit", func(s *task.State) { s.MaxAttemptCount = 0 }},
		{"negative limit", func(s *task.State) { s.MaxAttemptCount = -1 }},
		{"retry policy unsupported", func(s *task.State) { s.MaxAttemptCount = 2 }},
		{"negative count", func(s *task.State) { s.AttemptCount = -1 }},
		{"exhausted count", func(s *task.State) { s.AttemptCount = 2 }},
		{"overflow count", func(s *task.State) { s.AttemptCount = math.MaxInt32 }},
		{"negative attempt number", func(s *task.State) { s.LastAttempt.Number = -1 }},
		{"mismatched attempt number", func(s *task.State) { s.LastAttempt.Number = 2 }},
		{"unknown attempt status", func(s *task.State) { s.LastAttempt.Status = "UNKNOWN" }},
		{"unknown task status", func(s *task.State) { s.Status = "UNKNOWN" }},
	}
	for _, state := range states() {
		for _, tc := range cases {
			t.Run(string(state.Status)+"/"+tc.name, func(t *testing.T) {
				input := state
				tc.change(&input)
				assertRejected(t, input)
			})
		}
	}
	assertRejected(t, task.State{})
	for _, state := range states() {
		for _, other := range states() {
			// PENDING and READY both have no attempt; every other pairing is inconsistent.
			if state.LastAttempt == other.LastAttempt {
				continue
			}
			invalid := state
			invalid.LastAttempt = other.LastAttempt
			assertRejected(t, invalid)
		}
		invalid := state
		if state.AttemptCount == 0 {
			invalid.AttemptCount = 1
		} else {
			invalid.AttemptCount = 0
		}
		assertRejected(t, invalid)
	}
}

func assertRejected(t *testing.T, before task.State) {
	t.Helper()
	for _, event := range []task.Event{task.Activate, task.Claim, task.Succeed, task.Fail} {
		got, err := task.Transition(before, event)
		if !errors.Is(err, task.ErrInvalidTransition) || got != before {
			t.Fatalf("Transition(%+v, %s) = %+v, %v; want unchanged state and invalid_transition", before, event, got, err)
		}
	}
}

func TestLifecycleAndRepeatedCompletion(t *testing.T) {
	for _, outcome := range []task.Event{task.Succeed, task.Fail} {
		t.Run(string(outcome), func(t *testing.T) {
			state, err := task.NewState(1)
			if err != nil {
				t.Fatal(err)
			}
			for _, event := range []task.Event{task.Activate, task.Claim, outcome} {
				state, err = task.Transition(state, event)
				if err != nil {
					t.Fatal(err)
				}
			}
			want := states()[3]
			if outcome == task.Fail {
				want = states()[4]
			}
			if state != want {
				t.Fatalf("terminal state = %+v; want %+v", state, want)
			}
			assertRejected(t, state)
		})
	}
}
