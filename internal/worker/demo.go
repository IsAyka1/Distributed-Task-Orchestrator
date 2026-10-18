package worker

import (
	"context"
	"encoding/json"
	"errors"
	"math"

	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/workflow"
)

const DemoA, DemoB, DemoC = "A", "B", "C"

type DemoPayload struct {
	N    int64    `json:"n"`
	Keys []string `json:"keys"`
}
type DemoActivity struct{}

func (DemoActivity) Execute(ctx context.Context, input workflow.Payload, key string) (workflow.Payload, error) {
	if err := ctx.Err(); err != nil {
		return workflow.Payload{}, err
	}
	var value DemoPayload
	if err := json.Unmarshal(input.Value, &value); err != nil {
		return workflow.Payload{}, errors.New("invalid demo input")
	}
	if value.N == math.MaxInt64 {
		return workflow.Payload{}, errors.New("demo counter overflow")
	}
	value.N++
	value.Keys = append(value.Keys, key)
	output, err := json.Marshal(value)
	return workflow.Payload{Value: output}, err
}
