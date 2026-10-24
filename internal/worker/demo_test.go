package worker

import (
	"context"
	"encoding/json"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/workflow"
	"testing"
)

func TestDemoActivity(t *testing.T) {
	output, err := (DemoActivity{}).Execute(context.Background(), workflow.Payload{Value: []byte(`{"n":1}`)}, "task-id")
	if err != nil {
		t.Fatal(err)
	}
	var got DemoPayload
	if err := json.Unmarshal(output.Value, &got); err != nil {
		t.Fatal(err)
	}
	if got.N != 2 || len(got.Keys) != 1 || got.Keys[0] != "task-id" {
		t.Fatalf("output: %+v", got)
	}
	for _, input := range []string{`"invalid"`, `{`, `{"n":9223372036854775807}`} {
		if _, err := (DemoActivity{}).Execute(context.Background(), workflow.Payload{Value: []byte(input)}, "key"); err == nil {
			t.Fatal("accepted invalid input")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (DemoActivity{}).Execute(ctx, workflow.Payload{Value: []byte(`{}`)}, "key"); err == nil {
		t.Fatal("executed after cancellation")
	}
}
