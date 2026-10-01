package application

import (
	"context"
	"errors"
	"testing"

	"github.com/IsAyka1/Distributed-Task-Orchestrator/internal/workflow"
)

type definitionStoreFake struct {
	inserted *workflow.Definition
	key      DefinitionKey
	result   PublishedDefinition
	err      error
}

func (f *definitionStoreFake) Insert(ctx context.Context, d *workflow.Definition) (PublishedDefinition, error) {
	f.inserted = d
	return f.result, f.err
}
func (f *definitionStoreFake) Find(ctx context.Context, key DefinitionKey) (PublishedDefinition, error) {
	f.key = key
	return f.result, f.err
}
func validSpec() workflow.DefinitionSpec {
	return workflow.DefinitionSpec{Provider: "demo", Name: "flow", Version: 1, Tasks: []workflow.TaskSpec{{ID: "a", Type: "activity"}}}
}
func TestPublishRejectsInvalidBeforeStorage(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*workflow.DefinitionSpec)
	}{
		{"invalid UTF-8 task id", func(s *workflow.DefinitionSpec) { s.Tasks[0].ID = string([]byte{0xff}) }},
		{"NUL task id", func(s *workflow.DefinitionSpec) { s.Tasks[0].ID = "a\x00b" }},
		{"invalid UTF-8 provider", func(s *workflow.DefinitionSpec) { s.Provider = string([]byte{0xff}) }},
		{"NUL name", func(s *workflow.DefinitionSpec) { s.Name = "a\x00b" }},
		{"empty id", func(s *workflow.DefinitionSpec) { s.Tasks[0].ID = "" }},
		{"unsupported type", func(s *workflow.DefinitionSpec) { s.Tasks[0].Type = "timer" }},
		{"missing type", func(s *workflow.DefinitionSpec) { s.Tasks[0].Type = "" }},
		{"cycle", func(s *workflow.DefinitionSpec) { s.Tasks[0].DependsOn = []string{"a"} }},
		{"empty provider", func(s *workflow.DefinitionSpec) { s.Provider = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &definitionStoreFake{}
			spec := validSpec()
			tc.change(&spec)
			_, err := (Definitions{Store: f}).Publish(context.Background(), spec)
			if !errors.Is(err, workflow.ErrInvalidDefinition) || f.inserted != nil {
				t.Fatalf("err=%v inserted=%v", err, f.inserted)
			}
		})
	}
}
func TestPublishAllowsDAGAndPreservesPolicy(t *testing.T) {
	f := &definitionStoreFake{result: PublishedDefinition{ID: "published"}}
	spec := validSpec()
	spec.Name = " Flow "
	attempts := int64(3)
	spec.Tasks[0].MaxAttempts = &attempts
	spec.Tasks = append(spec.Tasks, workflow.TaskSpec{ID: "b", Type: "activity"})
	got, err := (Definitions{Store: f}).Publish(context.Background(), spec)
	if err != nil || got.ID != "published" || f.inserted.Name() != " Flow " || f.inserted.Tasks()[0].MaxAttempts != 3 {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	spec.Tasks[0].ID = "changed"
	attempts = 7
	if f.inserted.Tasks()[0].ID != "a" || f.inserted.Tasks()[0].MaxAttempts != 3 {
		t.Fatal("publication retained caller data")
	}
}
func TestDefinitionStorageErrors(t *testing.T) {
	f := &definitionStoreFake{err: ErrDefinitionExists}
	service := Definitions{Store: f}
	if _, err := service.Publish(context.Background(), validSpec()); !errors.Is(err, ErrDefinitionExists) {
		t.Fatal(err)
	}
	f.err = ErrDefinitionNotFound
	key := DefinitionKey{Provider: "demo", Name: "flow", Version: 9}
	if _, err := service.Get(context.Background(), key); !errors.Is(err, ErrDefinitionNotFound) || f.key != key {
		t.Fatalf("key=%+v err=%v", f.key, err)
	}
	f.key = DefinitionKey{}
	if _, err := service.Get(context.Background(), DefinitionKey{Version: -1}); !errors.Is(err, workflow.ErrInvalidDefinition) || f.key != (DefinitionKey{}) {
		t.Fatal(err)
	}
}
