// Package parser decodes a piperun.hcl file into a schema.Pipeline.
package parser

import (
	"fmt"
	"os"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"github.com/piperun/piperun/internal/schema"
)

// ----- raw HCL structs (mirrors the .hcl file shape) -----

type rawFile struct {
	Piperun   []rawPiperun  `hcl:"piperun,block"`
	Stages    []rawStage    `hcl:"stage,block"`
	Variables []rawVariable `hcl:"variable,block"`
	Locals    []rawLocals   `hcl:"locals,block"`
	Remain    hcl.Body      `hcl:",remain"`
}

type rawPiperun struct {
	Version int `hcl:"version"`
}

type rawStage struct {
	Name      string         `hcl:"name,label"`
	Script    *string        `hcl:"script,optional"`
	DependsOn *[]string      `hcl:"depends_on,optional"`
	If        *string        `hcl:"if,optional"`
	ForEach   *string        `hcl:"for_each,optional"`
	Lifecycle *[]string      `hcl:"lifecycle,optional"`
	Container []rawContainer `hcl:"container,block"`
	PreHook   []rawHook      `hcl:"pre_hook,block"`
	PostHook  []rawHook      `hcl:"post_hook,block"`
}

type rawContainer struct {
	Image string `hcl:"image"`
}

type rawHook struct {
	Script string `hcl:"script"`
}

type rawVariable struct {
	Name        string  `hcl:"name,label"`
	Type        *string `hcl:"type,optional"`
	Default     *string `hcl:"default,optional"`
	Description *string `hcl:"description,optional"`
}

type rawLocals struct {
	Remain hcl.Body `hcl:",remain"`
}

// decodeLocals extracts string attributes from a locals block body.
func decodeLocals(body hcl.Body, ctx *hcl.EvalContext) (map[string]string, error) {
	attrs, diags := body.JustAttributes()
	if diags.HasErrors() {
		return nil, diags
	}
	result := make(map[string]string, len(attrs))
	for name, attr := range attrs {
		val, diags := attr.Expr.Value(ctx)
		if diags.HasErrors() {
			return nil, fmt.Errorf("locals.%s: %w", name, diags)
		}
		if val.Type() == cty.String {
			result[name] = val.AsString()
		}
	}
	return result, nil
}

// ParseFile reads and decodes a piperun HCL file, returning a Pipeline.
func ParseFile(path string) (*schema.Pipeline, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return Parse(src, path)
}

// Parse decodes HCL source bytes into a Pipeline.
// It uses a two-pass approach:
//  1. Parse the raw HCL and collect variable/stage names.
//  2. Build an eval context with placeholder values for var.* and stage.*
//     so that HCL template interpolations succeed at decode time.
//     Actual values are substituted later at execution time by the engine.
func Parse(src []byte, filename string) (*schema.Pipeline, error) {
	parser := hclparse.NewParser()
	file, diags := parser.ParseHCL(src, filename)
	if diags.HasErrors() {
		return nil, fmt.Errorf("parse error: %s", diags.Error())
	}

	// --- Pass 1: quick scan for variable & stage names ---
	varNames, stageNames := scanNames(file.Body)

	// --- Build eval context with placeholders ---
	ctx := buildEvalContext(varNames, stageNames)

	// --- Pass 2: full decode ---
	var raw rawFile
	diags = gohcl.DecodeBody(file.Body, ctx, &raw)
	if diags.HasErrors() {
		return nil, fmt.Errorf("decode error: %s", diags.Error())
	}

	p := &schema.Pipeline{
		Stages:    make(map[string]*schema.Stage),
		Variables: make(map[string]*schema.Variable),
		Locals:    make(map[string]string),
	}

	// version
	if len(raw.Piperun) > 0 {
		p.Version = raw.Piperun[0].Version
	}

	// variables
	for _, v := range raw.Variables {
		sv := &schema.Variable{Name: v.Name}
		if v.Type != nil {
			sv.Type = *v.Type
		}
		if v.Default != nil {
			sv.Default = *v.Default
		}
		if v.Description != nil {
			sv.Description = *v.Description
		}
		p.Variables[v.Name] = sv
	}

	// locals
	for _, l := range raw.Locals {
		locals, err := decodeLocals(l.Remain, ctx)
		if err != nil {
			return nil, fmt.Errorf("decode locals: %w", err)
		}
		for k, v := range locals {
			p.Locals[k] = v
		}
	}

	// stages
	for _, s := range raw.Stages {
		st := &schema.Stage{Name: s.Name}
		if s.Script != nil {
			st.Script = *s.Script
		}
		if s.DependsOn != nil {
			// strip "stage." prefix if present
			for _, d := range *s.DependsOn {
				st.DependsOn = append(st.DependsOn, stripStagePrefix(d))
			}
		}
		if s.If != nil {
			st.If = *s.If
		}
		if s.ForEach != nil {
			st.ForEach = *s.ForEach
		}
		if s.Lifecycle != nil {
			st.Lifecycle = *s.Lifecycle
		}
		if len(s.Container) > 0 {
			st.Container = &schema.Container{Image: s.Container[0].Image}
		}
		if len(s.PreHook) > 0 {
			st.PreHook = &schema.Hook{Script: s.PreHook[0].Script}
		}
		if len(s.PostHook) > 0 {
			st.PostHook = &schema.Hook{Script: s.PostHook[0].Script}
		}
		p.Stages[s.Name] = st
	}

	return p, nil
}

func stripStagePrefix(s string) string {
	return strings.TrimPrefix(s, "stage.")
}

// scanNames walks the top-level HCL body and collects variable and stage label
// names so we can build placeholder values before full decode.
func scanNames(body hcl.Body) (varNames []string, stageNames []string) {
	syntaxBody, ok := body.(*hclsyntax.Body)
	if !ok {
		return nil, nil
	}
	for _, block := range syntaxBody.Blocks {
		switch block.Type {
		case "variable":
			if len(block.Labels) > 0 {
				varNames = append(varNames, block.Labels[0])
			}
		case "stage":
			if len(block.Labels) > 0 {
				stageNames = append(stageNames, block.Labels[0])
			}
		}
	}
	return
}

// buildEvalContext creates an HCL evaluation context that provides placeholder
// values for every known variable and stage reference. This lets HCL
// interpolation succeed at parse time; the engine replaces placeholders later.
func buildEvalContext(varNames, stageNames []string) *hcl.EvalContext {
	variables := make(map[string]cty.Value)

	// var.<name> → placeholder string
	if len(varNames) > 0 {
		vm := make(map[string]cty.Value)
		for _, n := range varNames {
			vm[n] = cty.StringVal(fmt.Sprintf("${var.%s}", n))
		}
		variables["var"] = cty.ObjectVal(vm)
	}

	// stage.<name> → placeholder string (enough for depends_on refs)
	if len(stageNames) > 0 {
		sm := make(map[string]cty.Value)
		for _, n := range stageNames {
			sm[n] = cty.StringVal(fmt.Sprintf("stage.%s", n))
		}
		variables["stage"] = cty.ObjectVal(sm)
	}

	// local.* — empty object for now (populated at runtime)
	variables["local"] = cty.EmptyObjectVal

	return &hcl.EvalContext{
		Variables: variables,
	}
}
