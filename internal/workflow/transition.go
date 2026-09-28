package workflow

import "errors"

type Status string

const (
	Pending   Status = "PENDING"
	Running   Status = "RUNNING"
	Succeeded Status = "SUCCEEDED"
	Failed    Status = "FAILED"
)

type Event string

const (
	Start   Event = "START"
	Succeed Event = "SUCCEED"
	Fail    Event = "FAIL"
)

var ErrInvalidTransition = errors.New("invalid_transition")

func Transition(status Status, event Event) (Status, error) {
	switch {
	case status == Pending && event == Start:
		return Running, nil
	case status == Running && event == Succeed:
		return Succeeded, nil
	case status == Running && event == Fail:
		return Failed, nil
	default:
		return status, ErrInvalidTransition
	}
}
