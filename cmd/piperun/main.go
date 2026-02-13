// piperun – a declarative pipeline orchestrator powered by HCL.
//
// Usage:
//
//	piperun [lifecycle] [flags] [+stage.name] [^stage.name]
//
// Lifecycles: default, build, deploy, test, or any custom name.
// Flags:
//
//	-f          path to pipeline file (default: piperun.hcl)
//	-dry-run    print what would run without executing
//	-var        set a variable: -var key=value
//	-fmt        format a pipeline file in-place
//	-validate   parse and validate without executing
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/piperun/piperun/internal/engine"
	"github.com/piperun/piperun/internal/formatter"
	"github.com/piperun/piperun/internal/parser"
)

const version = "0.1.0"

func main() {
	var (
		filePath   string
		dryRun     bool
		doFmt      bool
		doValidate bool
		showVer    bool
		vars       flagSlice
	)

	flag.StringVar(&filePath, "f", "piperun.hcl", "path to pipeline file")
	flag.BoolVar(&dryRun, "dry-run", false, "dry-run mode")
	flag.BoolVar(&doFmt, "fmt", false, "format the pipeline file")
	flag.BoolVar(&doValidate, "validate", false, "validate without executing")
	flag.BoolVar(&showVer, "version", false, "print version")
	flag.Var(&vars, "var", "set variable: key=value (repeatable)")
	flag.Parse()

	if showVer {
		fmt.Printf("piperun version %s\n", version)
		os.Exit(0)
	}

	// -fmt mode
	if doFmt {
		if err := formatter.FormatFile(filePath); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("formatted", filePath)
		os.Exit(0)
	}

	// Parse the pipeline
	pipeline, err := parser.ParseFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// -validate mode
	if doValidate {
		fmt.Printf("✓ %s is valid (%d stages, %d variables)\n", filePath, len(pipeline.Stages), len(pipeline.Variables))
		os.Exit(0)
	}

	// Determine lifecycle, allow/block from positional args
	lifecycle := "default"
	var allow, block []string
	for _, arg := range flag.Args() {
		if strings.HasPrefix(arg, "+stage.") {
			allow = append(allow, strings.TrimPrefix(arg, "+stage."))
		} else if strings.HasPrefix(arg, "^stage.") {
			block = append(block, strings.TrimPrefix(arg, "^stage."))
		} else {
			lifecycle = arg
		}
	}

	// Build variable map from -var flags and env
	varMap := make(map[string]string)
	for _, v := range vars {
		parts := strings.SplitN(v, "=", 2)
		if len(parts) == 2 {
			varMap[parts[0]] = parts[1]
		}
	}
	// Fill defaults for missing vars
	for k, vDef := range pipeline.Variables {
		if _, ok := varMap[k]; !ok && vDef.Default != "" {
			varMap[k] = vDef.Default
		}
	}

	eng := engine.New(pipeline)
	eng.Lifecycle = lifecycle
	eng.Allow = allow
	eng.Block = block
	eng.DryRun = dryRun
	eng.Variables = varMap

	ctx := context.Background()
	_, runErr := eng.Run(ctx)
	if runErr != nil {
		fmt.Fprintf(os.Stderr, "\nerror: %v\n", runErr)
		os.Exit(1)
	}
}

// flagSlice allows repeatable -var flags.
type flagSlice []string

func (f *flagSlice) String() string { return strings.Join(*f, ", ") }
func (f *flagSlice) Set(v string) error {
	*f = append(*f, v)
	return nil
}
