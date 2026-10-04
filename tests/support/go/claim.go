package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/services/queue"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/storage/postgres"
)

type claimInput struct {
	WorkerID      string `json:"worker_id"`
	LeaseDuration string `json:"lease_duration"`
}

func claimCommand(ctx context.Context, db *sql.DB) error {
	var input claimInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		return err
	}
	duration, err := time.ParseDuration(input.LeaseDuration)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	claimed, err := (queue.Service{Repository: postgres.ClaimRepository{DB: db}}).Claim(ctx, queue.ClaimRequest{
		WorkerID: input.WorkerID, LeaseDuration: duration})
	if errors.Is(err, queue.ErrNoTask) {
		return json.NewEncoder(os.Stdout).Encode(nil)
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(claimed)
}
