package workflow_test

import (
	"errors"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/workflow"
	"testing"
)

func TestWorkflowTransitions(t *testing.T) {
	for _, from := range []workflow.Status{workflow.Pending, workflow.Running, workflow.Succeeded, workflow.Failed, "", "UNKNOWN"} {
		for _, event := range []workflow.Event{workflow.Start, workflow.Succeed, workflow.Fail, "", "RESET"} {
			want := from
			allowed := false
			if from == workflow.Pending && event == workflow.Start {
				want = workflow.Running
				allowed = true
			}
			if from == workflow.Running && event == workflow.Succeed {
				want = workflow.Succeeded
				allowed = true
			}
			if from == workflow.Running && event == workflow.Fail {
				want = workflow.Failed
				allowed = true
			}
			got, err := workflow.Transition(from, event)
			if got != want || (allowed && err != nil) || (!allowed && !errors.Is(err, workflow.ErrInvalidTransition)) {
				t.Fatalf("Transition(%s,%s) = %s, %v; want %s", from, event, got, err, want)
			}
		}
	}
}
