package task

import "errors"

type Status string

const (
	Pending   Status = "PENDING"
	Ready     Status = "READY"
	Running   Status = "RUNNING"
	Succeeded Status = "SUCCEEDED"
	Failed    Status = "FAILED"
)

type AttemptStatus string

const (
	AttemptRunning   AttemptStatus = "RUNNING"
	AttemptSucceeded AttemptStatus = "SUCCEEDED"
	AttemptFailed    AttemptStatus = "FAILED"
)

type Event string

const (
	Activate Event = "ACTIVATE"
	Claim    Event = "CLAIM"
	Succeed  Event = "SUCCEED"
	Fail     Event = "FAIL"
)

var (
	ErrInvalidTransition = errors.New("invalid_transition")
	ErrUnsupportedPolicy = errors.New("unsupported_policy")
)

type Attempt struct {
	Number int32
	Status AttemptStatus
}

// State is a value snapshot for pure transitions, not a persisted task record.
type State struct {
	Status          Status
	MaxAttemptCount int32
	AttemptCount    int32
	LastAttempt     Attempt // Zero before claim; retains the outcome after closure.
}

func NewState(maxAttemptCount int32) (State, error) {
	if maxAttemptCount != 1 {
		return State{}, ErrUnsupportedPolicy
	}
	return State{Status: Pending, MaxAttemptCount: maxAttemptCount}, nil
}

// Transition returns a proposal; callers own eligibility, fencing and atomic persistence.
// On rejection, it returns the original snapshot and ErrInvalidTransition.
func Transition(state State, event Event) (State, error) {
	if !state.valid() {
		return state, ErrInvalidTransition
	}
	next := state
	switch {
	case state.Status == Pending && event == Activate:
		next.Status = Ready
	case state.Status == Ready && event == Claim:
		next.Status = Running
		next.AttemptCount++
		next.LastAttempt = Attempt{Number: next.AttemptCount, Status: AttemptRunning}
	case state.Status == Running && event == Succeed:
		next.Status = Succeeded
		next.LastAttempt.Status = AttemptSucceeded
	case state.Status == Running && event == Fail:
		next.Status = Failed
		next.LastAttempt.Status = AttemptFailed
	default:
		return state, ErrInvalidTransition
	}
	return next, nil
}

func (s State) valid() bool {
	// Retry policies are enabled separately in stage 4.
	if s.MaxAttemptCount != 1 {
		return false
	}
	switch s.Status {
	case Pending, Ready:
		return s.AttemptCount == 0 && s.LastAttempt == (Attempt{})
	case Running, Succeeded, Failed:
		if s.AttemptCount != 1 || s.LastAttempt.Number != s.AttemptCount {
			return false
		}
		return (s.Status == Running && s.LastAttempt.Status == AttemptRunning) ||
			(s.Status == Succeeded && s.LastAttempt.Status == AttemptSucceeded) ||
			(s.Status == Failed && s.LastAttempt.Status == AttemptFailed)
	default:
		return false
	}
}
