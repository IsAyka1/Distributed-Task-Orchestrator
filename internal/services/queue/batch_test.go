package queue

import (
	"context"
	"errors"
	"fmt"
	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/task"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBatchRejectsInvalidLimitBeforeOpeningTransaction(t *testing.T) {
	for _, limit := range []int{-1, 0, 101} {
		got, err := (Service{}).ClaimBatch(context.Background(), BatchClaimRequest{
			ClaimRequest: ClaimRequest{WorkerID: "worker", LeaseDuration: time.Minute}, Limit: limit,
		})
		if !errors.Is(err, ErrInvalidRequest) || len(got) != 0 {
			t.Fatalf("limit %d: claim=%v err=%v", limit, got, err)
		}
	}
}

func batchRequest(limit int) BatchClaimRequest {
	return BatchClaimRequest{ClaimRequest: ClaimRequest{WorkerID: "worker", LeaseDuration: time.Minute}, Limit: limit}
}

func TestBatchCommitsAllClaimsOnce(t *testing.T) {
	for _, limit := range []int{1, 3, MaxBatchSize} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			f := newClaimFake()
			for i := 0; i < limit; i++ {
				candidate := f.candidate
				candidate.ID = fmt.Sprint("task-", i)
				candidate.WorkflowID = fmt.Sprint("workflow-", i)
				f.candidates = append(f.candidates, candidate)
			}
			got, err := (Service{Repository: f}).ClaimBatch(context.Background(), batchRequest(limit))
			if err != nil || len(got) != limit || f.limit != limit {
				t.Fatalf("claim=%v limit=%d err=%v", got, f.limit, err)
			}
			if !reflect.DeepEqual(got, f.updatedAll) {
				t.Fatalf("persisted claims differ: %v", f.updatedAll)
			}
			for i, claimed := range got {
				if claimed.TaskID != f.candidates[i].ID || claimed.WorkflowID != f.candidates[i].WorkflowID ||
					claimed.Attempt.TaskID != claimed.TaskID || claimed.Attempt.Number != 1 ||
					!claimed.LeaseExpiresAt.Equal(f.now.Add(time.Minute)) {
					t.Fatalf("claim: %+v", claimed)
				}
			}
			if strings.Count(strings.Join(f.calls, ","), "commit") != 1 {
				t.Fatal(f.calls)
			}
		})
	}
}

func TestEmptyBatchDoesNotWrite(t *testing.T) {
	f := newClaimFake()
	f.candidates = []Candidate{}
	got, err := (Service{Repository: f}).ClaimBatch(context.Background(), batchRequest(3))
	if err != nil || got == nil || len(got) != 0 || !reflect.DeepEqual(f.calls, []string{"begin", "lock", "rollback"}) {
		t.Fatalf("claim=%v err=%v calls=%v", got, err, f.calls)
	}
}

func TestBatchRechecksAllCandidatesAndReturnsNoPartialResult(t *testing.T) {
	for _, stage := range []string{"update", "enqueue", "revision", "commit", "transition"} {
		t.Run(stage, func(t *testing.T) {
			f := newClaimFake()
			f.candidates = []Candidate{f.candidate, f.candidate}
			f.candidates[1].ID, f.candidates[1].WorkflowID = "second-task", "second-workflow"
			f.fail, f.failAt = stage, 2
			want := f.failure
			if stage == "transition" {
				f.candidates[1].State.Status = task.Pending
				want = task.ErrInvalidTransition
			}
			got, err := (Service{Repository: f}).ClaimBatch(context.Background(), batchRequest(2))
			if !errors.Is(err, want) || got != nil || f.calls[len(f.calls)-1] != "rollback" {
				t.Fatalf("claim=%v err=%v calls=%v", got, err, f.calls)
			}
			if len(f.updatedAll) == 0 {
				t.Fatal("failure must happen after the first provisional claim")
			}
		})
	}
}

func TestBatchOmitsFutureCandidateWithoutDiscardingEligibleWork(t *testing.T) {
	f := newClaimFake()
	future := f.candidate
	future.ID, future.WorkflowID, future.AvailableAt = "future-task", "future-workflow", f.now.Add(time.Second)
	f.candidates = []Candidate{future, f.candidate}
	got, err := (Service{Repository: f}).ClaimBatch(context.Background(), batchRequest(2))
	if err != nil || len(got) != 1 || got[0].TaskID != f.candidate.ID {
		t.Fatalf("claim=%v err=%v", got, err)
	}
}
