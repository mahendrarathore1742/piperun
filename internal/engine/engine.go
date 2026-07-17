// Package engine runs a parsed pipeline: builds the DAG, resolves layers,
// and executes stages concurrently within each layer.
package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
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
	name := st.Name
	e.Log.Info(name, fmt.Sprintf("[+] %s", name))

	return e.executeSingleStage(ctx, st, name, nil)
}

// executeSingleStage runs the actual stage logic (hooks, condition, script/container).
func (e *Engine) executeSingleStage(ctx context.Context, st *schema.Stage, name string, eachValue *string) Result {
	start := time.Now()

	// Pre-hook
	if st.PreHook != nil {
		hookScript := e.interpolateWithEach(st.PreHook.Script, eachValue)
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
			e.Log.Info(name, "condition false -> skipped")
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
		script := e.interpolateWithEach(st.Script, eachValue)
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
		hookScript := e.interpolateWithEach(st.PostHook.Script, eachValue)
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

	script := e.interpolateWithEach(st.Script, nil)
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

// interpolateWithEach performs ${var.xxx}, ${local.xxx}, and ${each.value} substitution.
func (e *Engine) interpolateWithEach(s string, eachValue *string) string {
	result := s
	// Variable interpolation
	for k, v := range e.Variables {
		result = strings.ReplaceAll(result, fmt.Sprintf("${var.%s}", k), v)
	}
	// Locals interpolation
	for k, v := range e.Pipeline.Locals {
		result = strings.ReplaceAll(result, fmt.Sprintf("${local.%s}", k), v)
	}
	// each.value interpolation
	if eachValue != nil {
		result = strings.ReplaceAll(result, "${each.value}", *eachValue)
	}
	return result
}

// evalCondition evaluates a condition expression.
// Supported patterns:
//
//	env("KEY") == "value"   - env var equals value
//	env("KEY") != "value"   - env var not equal to value
//	env("KEY") > "value"    - env var greater than value
//	env("KEY") < "value"    - env var less than value
//	env("KEY") >= "value"   - env var greater than or equal
//	env("KEY") <= "value"   - env var less than or equal
//	env("KEY") =~ "pattern" - env var matches regex
//	env("KEY") !~ "pattern" - env var does not match regex
//	var("KEY") == "value"   - variable equals value (same operators as env)
//	!expr                   - negation
//	expr1 && expr2          - logical AND
//	expr1 || expr2          - logical OR
//	env("KEY")              - truthy check (non-empty, non-"false", non-"0")
func (e *Engine) evalCondition(expr string) bool {
	expr = strings.TrimSpace(expr)

	// Handle || (OR) - lowest precedence
	if idx := findOperatorOutsideParens(expr, "||"); idx >= 0 {
		left := expr[:idx]
		right := expr[idx+2:]
		return e.evalCondition(left) || e.evalCondition(right)
	}

	// Handle && (AND) - higher precedence than ||
	if idx := findOperatorOutsideParens(expr, "&&"); idx >= 0 {
		left := expr[:idx]
		right := expr[idx+2:]
		return e.evalCondition(left) && e.evalCondition(right)
	}

	// Handle negation: !expr
	if strings.HasPrefix(expr, "!") {
		return !e.evalCondition(expr[1:])
	}

	// Handle parenthesized expressions: (expr)
	if strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") {
		return e.evalCondition(expr[1 : len(expr)-1])
	}

	// Try binary operators: =~ !~ == != >= <= > <
	for _, op := range []string{"=~", "!~", "==", "!=", ">=", "<=", ">", "<"} {
		if idx := findOperatorOutsideParens(expr, op); idx >= 0 {
			left := strings.TrimSpace(expr[:idx])
			right := strings.TrimSpace(expr[idx+len(op):])
			return e.evalBinaryOp(left, op, right)
		}
	}

	// Truthy check for env("KEY") or var("KEY")
	return e.resolveValue(expr)
}

// findOperatorOutsideParens finds the last occurrence of op that is not inside
// parentheses or quotes, returning its index or -1 if not found.
func findOperatorOutsideParens(expr, op string) int {
	depth := 0
	inQuote := false
	quoteChar := byte(0)

	for i := len(expr) - 1; i >= 0; i-- {
		c := expr[i]

		if inQuote {
			if c == quoteChar && (i == 0 || expr[i-1] != '\\') {
				inQuote = false
			}
			continue
		}

		switch c {
		case '"', '\'':
			inQuote = true
			quoteChar = c
		case ')':
			depth++
		case '(':
			depth--
		default:
			if depth == 0 && i >= len(op)-1 {
				matched := true
				for j := 0; j < len(op); j++ {
					if expr[i-(len(op)-1)+j] != op[j] {
						matched = false
						break
					}
				}
				if matched {
					if (op == ">" || op == "<") && i > 0 {
						prev := expr[i-1]
						if prev == '=' || prev == '!' {
							continue
						}
					}
					return i - (len(op) - 1)
				}
			}
		}
	}
	return -1
}

// evalBinaryOp evaluates a binary operation like left op right.
func (e *Engine) evalBinaryOp(left, op, right string) bool {
	leftVal := e.resolveValueRaw(left)
	rightVal := e.resolveValueRaw(right)

	if leftNum, err1 := strconv.ParseFloat(leftVal, 64); err1 == nil {
		if rightNum, err2 := strconv.ParseFloat(rightVal, 64); err2 == nil {
			switch op {
			case "==":
				return leftNum == rightNum
			case "!=":
				return leftNum != rightNum
			case ">":
				return leftNum > rightNum
			case "<":
				return leftNum < rightNum
			case ">=":
				return leftNum >= rightNum
			case "<=":
				return leftNum <= rightNum
			}
		}
	}

	leftStr := strings.Trim(leftVal, "\"")
	rightStr := strings.Trim(rightVal, "\"")

	switch op {
	case "==":
		return leftStr == rightStr
	case "!=":
		return leftStr != rightStr
	case ">":
		return leftStr > rightStr
	case "<":
		return leftStr < rightStr
	case ">=":
		return leftStr >= rightStr
	case "<=":
		return leftStr <= rightStr
	case "=~":
		re, err := regexp.Compile(rightStr)
		if err != nil {
			return false
		}
		return re.MatchString(leftStr)
	case "!~":
		re, err := regexp.Compile(rightStr)
		if err != nil {
			return true
		}
		return !re.MatchString(leftStr)
	}

	return false
}

// resolveValue returns true if the expression is truthy.
func (e *Engine) resolveValue(expr string) bool {
	val := e.resolveValueRaw(expr)
	return val != "" && val != "false" && val != "0"
}

// resolveValueRaw returns the raw string value of an expression.
func (e *Engine) resolveValueRaw(expr string) string {
	expr = strings.TrimSpace(expr)

	// env("KEY")
	if strings.HasPrefix(expr, "env(\"") && strings.HasSuffix(expr, "\")") {
		key := expr[5 : len(expr)-2]
		return os.Getenv(key)
	}

	// var("KEY")
	if strings.HasPrefix(expr, "var(\"") && strings.HasSuffix(expr, "\")") {
		key := expr[5 : len(expr)-2]
		if val, ok := e.Variables[key]; ok {
			return val
		}
		return ""
	}

	// Strip quotes if present
	if strings.HasPrefix(expr, "\"") && strings.HasSuffix(expr, "\"") {
		return expr[1 : len(expr)-1]
	}

	return expr
}
