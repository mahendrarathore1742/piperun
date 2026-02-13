// Package engine runs a parsed pipeline: builds the DAG, resolves layers,
// and executes stages concurrently within each layer.
package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/piperun/piperun/internal/dag"
	"github.com/piperun/piperun/internal/logger"
	"github.com/piperun/piperun/internal/schema"
)

// Result captures the outcome of a single stage execution.
type Result struct {
	Stage    string
	Status   string // "success", "failed", "skipped"
	Duration time.Duration
	Err      error
}

// Engine orchestrates pipeline execution.
type Engine struct {
	Pipeline  *schema.Pipeline
	Log       *logger.Logger
	Lifecycle string   // e.g. "default", "build", "deploy"
	Allow     []string // +stage.x inclusions
	Block     []string // ^stage.x exclusions
	DryRun    bool
	Variables map[string]string // CLI-provided variable overrides
}

// New creates a new Engine for the given pipeline.
func New(p *schema.Pipeline) *Engine {
	return &Engine{
		Pipeline:  p,
		Log:       logger.New(),
		Lifecycle: "default",
		Variables: make(map[string]string),
	}
}

// Run executes the full pipeline.
func (e *Engine) Run(ctx context.Context) ([]Result, error) {
	e.Log.System(fmt.Sprintf("piperun (version=%d, lifecycle=%s)", e.Pipeline.Version, e.Lifecycle))

	// Build DAG
	g := dag.New()
	for name, st := range e.Pipeline.Stages {
		g.AddNode(name, st.DependsOn)
	}

	layers, err := g.Resolve()
	if err != nil {
		return nil, fmt.Errorf("dag resolve: %w", err)
	}

	var allResults []Result

	for _, layer := range layers {
		var wg sync.WaitGroup
		var mu sync.Mutex
		layerResults := make([]Result, 0, len(layer.Stages))

		for _, stageName := range layer.Stages {
			stage := e.Pipeline.Stages[stageName]
			if stage == nil {
				continue
			}

			// Lifecycle filtering
			if !e.shouldRun(stage) {
				e.Log.Info(stageName, "skipped")
				mu.Lock()
				layerResults = append(layerResults, Result{Stage: stageName, Status: "skipped"})
				mu.Unlock()
				continue
			}

			wg.Add(1)
			go func(st *schema.Stage) {
				defer wg.Done()
				res := e.executeStage(ctx, st)
				mu.Lock()
				layerResults = append(layerResults, res)
				mu.Unlock()
			}(stage)
		}
		wg.Wait()
		allResults = append(allResults, layerResults...)

		// If any stage in the layer failed, abort
		for _, r := range layerResults {
			if r.Status == "failed" {
				e.Log.Done()
				return allResults, fmt.Errorf("stage %q failed: %v", r.Stage, r.Err)
			}
		}
	}

	e.Log.Done()
	return allResults, nil
}

// shouldRun decides if a stage should execute based on lifecycle, allow/block lists.
func (e *Engine) shouldRun(st *schema.Stage) bool {
	// Explicit block
	for _, b := range e.Block {
		if b == st.Name {
			return false
		}
	}
	// If allow list is non-empty, stage must be in it
	if len(e.Allow) > 0 {
		for _, a := range e.Allow {
			if a == st.Name {
				return true
			}
		}
		return false
	}
	// Lifecycle filtering
	if len(st.Lifecycle) > 0 {
		for _, l := range st.Lifecycle {
			if l == e.Lifecycle || l == "all" {
				return true
			}
		}
		return false
	}
	return true
}

// executeStage runs a single stage (with hooks).
func (e *Engine) executeStage(ctx context.Context, st *schema.Stage) Result {
	start := time.Now()
	name := st.Name
	e.Log.Info(name, fmt.Sprintf("[+] %s", name))

	// Pre-hook
	if st.PreHook != nil {
		hookScript := e.interpolate(st.PreHook.Script)
		if e.DryRun {
			e.Log.Info(name, fmt.Sprintf("(dry-run) pre_hook: %s", hookScript))
		} else {
			e.Log.Info(name, fmt.Sprintf("pre_hook: %s", hookScript))
			if err := e.runScript(ctx, name, hookScript); err != nil {
				e.Log.Error(name, fmt.Sprintf("pre_hook failed: %v", err))
				return Result{Stage: name, Status: "failed", Duration: time.Since(start), Err: err}
			}
		}
	}

	// Conditional
	if st.If != "" {
		if !e.evalCondition(st.If) {
			e.Log.Info(name, "condition false → skipped")
			return Result{Stage: name, Status: "skipped", Duration: time.Since(start)}
		}
	}

	// Container-based execution
	if st.Container != nil {
		if err := e.runContainer(ctx, st); err != nil {
			e.Log.Error(name, fmt.Sprintf("failed: %v", err))
			return Result{Stage: name, Status: "failed", Duration: time.Since(start), Err: err}
		}
	} else if st.Script != "" {
		// Script-based execution
		script := e.interpolate(st.Script)
		if e.DryRun {
			e.Log.Info(name, fmt.Sprintf("(dry-run) %s", script))
		} else {
			if err := e.runScript(ctx, name, script); err != nil {
				e.Log.Error(name, fmt.Sprintf("failed: %v", err))
				return Result{Stage: name, Status: "failed", Duration: time.Since(start), Err: err}
			}
		}
	}

	// Post-hook
	if st.PostHook != nil {
		hookScript := e.interpolate(st.PostHook.Script)
		if e.DryRun {
			e.Log.Info(name, fmt.Sprintf("(dry-run) post_hook: %s", hookScript))
		} else {
			e.Log.Info(name, fmt.Sprintf("post_hook: %s", hookScript))
			if err := e.runScript(ctx, name, hookScript); err != nil {
				e.Log.Error(name, fmt.Sprintf("post_hook failed: %v", err))
				return Result{Stage: name, Status: "failed", Duration: time.Since(start), Err: err}
			}
		}
	}

	return Result{Stage: name, Status: "success", Duration: time.Since(start)}
}

// runScript executes a shell script line.
func (e *Engine) runScript(ctx context.Context, stage, script string) error {
	cmd := exec.CommandContext(ctx, "sh", "-c", script)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if len(out) > 0 {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			e.Log.Info(stage, line)
		}
	}
	return err
}

// runContainer runs a stage inside a docker container.
func (e *Engine) runContainer(ctx context.Context, st *schema.Stage) error {
	image := st.Container.Image
	e.Log.Info(st.Name, fmt.Sprintf("🐳 pulling %s", image))

	pullCmd := exec.CommandContext(ctx, "docker", "pull", image)
	if out, err := pullCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker pull %s: %s (%w)", image, string(out), err)
	}

	script := e.interpolate(st.Script)
	args := []string{"run", "--rm", image, "sh", "-c", script}
	runCmd := exec.CommandContext(ctx, "docker", args...)
	runCmd.Env = os.Environ()
	out, err := runCmd.CombinedOutput()
	if len(out) > 0 {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			e.Log.Info(st.Name, fmt.Sprintf("🐳 %s", line))
		}
	}
	return err
}

// interpolate performs simple ${var.xxx} and ${local.xxx} substitution.
func (e *Engine) interpolate(s string) string {
	result := s
	// Variable interpolation
	for k, v := range e.Variables {
		result = strings.ReplaceAll(result, fmt.Sprintf("${var.%s}", k), v)
	}
	// Locals interpolation
	for k, v := range e.Pipeline.Locals {
		result = strings.ReplaceAll(result, fmt.Sprintf("${local.%s}", k), v)
	}
	return result
}

// evalCondition evaluates a simple string condition (very basic for now).
func (e *Engine) evalCondition(expr string) bool {
	// Simple environment variable equality check: env("KEY") == "value"
	expr = strings.TrimSpace(expr)
	if strings.Contains(expr, "==") {
		parts := strings.SplitN(expr, "==", 2)
		left := strings.TrimSpace(parts[0])
		right := strings.Trim(strings.TrimSpace(parts[1]), "\"")

		if strings.HasPrefix(left, "env(") {
			key := strings.Trim(strings.TrimPrefix(left, "env("), "\")")
			return os.Getenv(key) == right
		}
	}
	// Default: run
	return true
}
