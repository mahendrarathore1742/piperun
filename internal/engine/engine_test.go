package engine

import (
	"context"
	"testing"

	"github.com/piperun/piperun/internal/schema"
)

func TestEngineSimple(t *testing.T) {
	p := &schema.Pipeline{
		Version: 2,
		Stages: map[string]*schema.Stage{
			"hello": {
				Name:   "hello",
				Script: "echo hello from piperun",
			},
		},
		Variables: make(map[string]*schema.Variable),
		Locals:    make(map[string]string),
	}

	eng := New(p)
	results, err := eng.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "success" {
		t.Errorf("expected success, got %s", results[0].Status)
	}
}

func TestEngineDependencyOrder(t *testing.T) {
	p := &schema.Pipeline{
		Version: 2,
		Stages: map[string]*schema.Stage{
			"first": {Name: "first", Script: "echo first"},
			"second": {
				Name:      "second",
				Script:    "echo second",
				DependsOn: []string{"first"},
			},
		},
		Variables: make(map[string]*schema.Variable),
		Locals:    make(map[string]string),
	}

	eng := New(p)
	results, err := eng.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// Both should succeed
	for _, r := range results {
		if r.Status != "success" {
			t.Errorf("stage %s: expected success, got %s", r.Stage, r.Status)
		}
	}
}

func TestEngineLifecycleSkip(t *testing.T) {
	p := &schema.Pipeline{
		Version: 2,
		Stages: map[string]*schema.Stage{
			"deploy_only": {
				Name:      "deploy_only",
				Script:    "echo deploy",
				Lifecycle: []string{"deploy"},
			},
		},
		Variables: make(map[string]*schema.Variable),
		Locals:    make(map[string]string),
	}

	eng := New(p)
	eng.Lifecycle = "build" // not "deploy", so it should be skipped
	results, err := eng.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Status != "skipped" {
		t.Errorf("expected skipped, got %s", results[0].Status)
	}
}

func TestEngineBlockList(t *testing.T) {
	p := &schema.Pipeline{
		Version: 2,
		Stages: map[string]*schema.Stage{
			"a": {Name: "a", Script: "echo a"},
			"b": {Name: "b", Script: "echo b"},
		},
		Variables: make(map[string]*schema.Variable),
		Locals:    make(map[string]string),
	}

	eng := New(p)
	eng.Block = []string{"b"}
	results, err := eng.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.Stage == "b" && r.Status != "skipped" {
			t.Errorf("expected b to be skipped, got %s", r.Status)
		}
	}
}
