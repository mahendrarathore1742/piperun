package parser

import (
	"testing"
)

const sampleHCL = `
piperun {
  version = 2
}

variable "name" {
  type    = "string"
  default = "world"
}

stage "greet" {
  script = "echo hello"
}

stage "build" {
  depends_on = ["stage.greet"]
  script     = "echo building"
}
`

func TestParseSample(t *testing.T) {
	p, err := Parse([]byte(sampleHCL), "test.hcl")
	if err != nil {
		t.Fatal(err)
	}
	if p.Version != 2 {
		t.Errorf("expected version 2, got %d", p.Version)
	}
	if len(p.Stages) != 2 {
		t.Errorf("expected 2 stages, got %d", len(p.Stages))
	}
	if len(p.Variables) != 1 {
		t.Errorf("expected 1 variable, got %d", len(p.Variables))
	}
	// depends_on should strip "stage." prefix
	build := p.Stages["build"]
	if build == nil {
		t.Fatal("missing stage 'build'")
	}
	if len(build.DependsOn) != 1 || build.DependsOn[0] != "greet" {
		t.Errorf("expected depends_on=[greet], got %v", build.DependsOn)
	}
}

func TestParseInvalid(t *testing.T) {
	_, err := Parse([]byte("this is not valid hcl!!!"), "bad.hcl")
	if err == nil {
		t.Fatal("expected error for invalid HCL")
	}
}
