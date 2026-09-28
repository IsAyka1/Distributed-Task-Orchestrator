package main

import (
	"context"
	"strings"
	"testing"
)

func TestRunRejectsInvalidInput(t *testing.T) {
	for _, args := range [][]string{nil, {"serve"}, {"migrate", "extra"}} {
		if err := run(context.Background(), args); err == nil || !strings.Contains(err.Error(), "usage:") {
			t.Fatalf("args %v: %v", args, err)
		}
	}
	t.Setenv("DATABASE_URL", "")
	if err := run(context.Background(), []string{"db-check"}); err == nil || err.Error() != "DATABASE_URL is required" {
		t.Fatalf("error = %v", err)
	}
}
