package engine

import (
	"context"
	"os"
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

func TestEvalConditionEnvEqual(t *testing.T) {
	eng := New(&schema.Pipeline{
		Variables: make(map[string]*schema.Variable),
		Locals:    make(map[string]string),
	})

	os.Setenv("TEST_COND_EQ", "hello")
	defer os.Unsetenv("TEST_COND_EQ")

	if !eng.evalCondition(`env("TEST_COND_EQ") == "hello"`) {
		t.Error("expected true for env == hello")
	}
	if eng.evalCondition(`env("TEST_COND_EQ") == "world"`) {
		t.Error("expected false for env == world")
	}
}

func TestEvalConditionEnvNotEqual(t *testing.T) {
	eng := New(&schema.Pipeline{
		Variables: make(map[string]*schema.Variable),
		Locals:    make(map[string]string),
	})

	os.Setenv("TEST_COND_NEQ", "hello")
	defer os.Unsetenv("TEST_COND_NEQ")

	if eng.evalCondition(`env("TEST_COND_NEQ") != "hello"`) {
		t.Error("expected false for env != hello")
	}
	if !eng.evalCondition(`env("TEST_COND_NEQ") != "world"`) {
		t.Error("expected true for env != world")
	}
}

func TestEvalConditionVarEqual(t *testing.T) {
	eng := New(&schema.Pipeline{
		Variables: map[string]*schema.Variable{},
		Locals:    make(map[string]string),
	})
	eng.Variables["debug"] = "true"

	if !eng.evalCondition(`var("debug") == "true"`) {
		t.Error("expected true for var debug == true")
	}
	if eng.evalCondition(`var("debug") == "false"`) {
		t.Error("expected false for var debug == false")
	}
}

func TestEvalConditionNumericComparison(t *testing.T) {
	eng := New(&schema.Pipeline{
		Variables: map[string]*schema.Variable{},
		Locals:    make(map[string]string),
	})
	eng.Variables["port"] = "8080"

	if !eng.evalCondition(`var("port") > "1024"`) {
		t.Error("expected 8080 > 1024")
	}
	if eng.evalCondition(`var("port") < "1024"`) {
		t.Error("expected 8080 not < 1024")
	}
	if !eng.evalCondition(`var("port") >= "8080"`) {
		t.Error("expected 8080 >= 8080")
	}
	if !eng.evalCondition(`var("port") <= "8080"`) {
		t.Error("expected 8080 <= 8080")
	}
}

func TestEvalConditionRegex(t *testing.T) {
	eng := New(&schema.Pipeline{
		Variables: make(map[string]*schema.Variable),
		Locals:    make(map[string]string),
	})

	os.Setenv("TEST_REGEX", "feature-123")
	defer os.Unsetenv("TEST_REGEX")

	if !eng.evalCondition(`env("TEST_REGEX") =~ "feature-\d+"`) {
		t.Error("expected regex match")
	}
	if eng.evalCondition(`env("TEST_REGEX") =~ "^release-"`) {
		t.Error("expected no regex match")
	}
	if !eng.evalCondition(`env("TEST_REGEX") !~ "^release-"`) {
		t.Error("expected regex not match")
	}
}

func TestEvalConditionLogicalOperators(t *testing.T) {
	eng := New(&schema.Pipeline{
		Variables: make(map[string]*schema.Variable),
		Locals:    make(map[string]string),
	})

	os.Setenv("TEST_CI", "true")
	os.Setenv("TEST_BRANCH", "main")
	defer os.Unsetenv("TEST_CI")
	defer os.Unsetenv("TEST_BRANCH")

	// AND
	if !eng.evalCondition(`env("TEST_CI") == "true" && env("TEST_BRANCH") == "main"`) {
		t.Error("expected AND to be true")
	}
	if eng.evalCondition(`env("TEST_CI") == "true" && env("TEST_BRANCH") == "dev"`) {
		t.Error("expected AND to be false")
	}

	// OR
	if !eng.evalCondition(`env("TEST_CI") == "true" || env("TEST_BRANCH") == "dev"`) {
		t.Error("expected OR to be true")
	}
	if eng.evalCondition(`env("TEST_CI") == "false" || env("TEST_BRANCH") == "dev"`) {
		t.Error("expected OR to be false")
	}
}

func TestEvalConditionNegation(t *testing.T) {
	eng := New(&schema.Pipeline{
		Variables: make(map[string]*schema.Variable),
		Locals:    make(map[string]string),
	})

	os.Setenv("TEST_NEG", "true")
	defer os.Unsetenv("TEST_NEG")

	if eng.evalCondition(`!env("TEST_NEG") == "true"`) {
		t.Error("expected negation to be false")
	}
	if !eng.evalCondition(`!env("TEST_NEG") == "false"`) {
		t.Error("expected negation to be true")
	}
}

func TestEvalConditionTruthy(t *testing.T) {
	eng := New(&schema.Pipeline{
		Variables: make(map[string]*schema.Variable),
		Locals:    make(map[string]string),
	})

	os.Setenv("TEST_TRUTHY", "yes")
	defer os.Unsetenv("TEST_TRUTHY")

	if !eng.evalCondition(`env("TEST_TRUTHY")`) {
		t.Error("expected truthy check to be true")
	}

	os.Setenv("TEST_FALSY", "false")
	if eng.evalCondition(`env("TEST_FALSY")`) {
		t.Error("expected 'false' to be falsy")
	}

	os.Setenv("TEST_ZERO", "0")
	if eng.evalCondition(`env("TEST_ZERO")`) {
		t.Error("expected '0' to be falsy")
	}
}
