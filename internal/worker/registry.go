package worker

import (
	"context"
	"errors"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/workflow"
)

type Activity interface {
	Execute(context.Context, workflow.Payload, string) (workflow.Payload, error)
}
type Registration struct {
	Key      string
	Activity Activity
}
type Registry struct{ activities map[string]Activity }

func NewRegistry(entries ...Registration) (Registry, error) {
	r := Registry{activities: make(map[string]Activity, len(entries))}
	for _, entry := range entries {
		if entry.Key == "" || entry.Activity == nil || r.activities[entry.Key] != nil {
			return Registry{}, errors.New("invalid or duplicate activity registration")
		}
		r.activities[entry.Key] = entry.Activity
	}
	return r, nil
}
