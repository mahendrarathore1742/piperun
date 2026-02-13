# 🚀 piperun

[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

**A declarative, parallel-by-default pipeline orchestrator powered by HCL.**

piperun lets you define build, test, and deploy pipelines in `.hcl` files using a familiar, Terraform-inspired syntax. Stages run concurrently by default; add `depends_on` when ordering matters.

```
$ piperun
0ms • piperun (version=2, lifecycle=default)
0ms • [stage.lint]  [+] lint
0ms • [stage.test]  [+] test
0ms • [stage.lint]  Running lint for myapp...
1ms • [stage.test]  Running tests for myapp...
2ms • [stage.build]  [+] build
2ms • [stage.build]  Building myapp...
3ms • took 3ms
```

---

## ✨ Features

| Feature | Description |
|---|---|
| **Parallel by default** | All stages in the same dependency layer run concurrently. |
| **DAG dependency resolution** | Topological sort with cycle detection. |
| **HCL configuration** | Declarative `.hcl` files with variables, locals, interpolation. |
| **Lifecycle & rule engine** | `piperun build`, `piperun deploy`, `+stage.x` / `^stage.x`. |
| **Pre/post hooks** | Run commands before and after any stage. |
| **Conditional execution** | `if` meta-argument with expression evaluation. |
| **Container support** | Run stages inside Docker containers. |
| **Formatter** | `piperun -fmt` to canonically format pipeline files. |
| **Validator** | `piperun -validate` to check syntax without running. |
| **Dry-run mode** | `piperun -dry-run` shows what would execute. |
| **Cross-platform** | Linux, macOS, Windows. Binaries via GoReleaser. |

---

## 📦 Installation

### Pre-built binaries

Download from [Releases](https://github.com/piperun/piperun/releases).

### Go install

```bash
go install github.com/piperun/piperun/cmd/piperun@latest
```

### Build from source

```bash
git clone https://github.com/piperun/piperun.git
cd piperun/cmd/piperun
go build -o piperun .
./piperun --help
```

### Docker

```bash
docker run --rm -v "$PWD":/work -w /work ghcr.io/piperun/piperun:latest
```

---

## 🏁 Quick Start

Create a file called `piperun.hcl`:

```hcl
piperun {
  version = 2
}

stage "hello" {
  script = "echo Hello from piperun!"
}
```

Run it:

```bash
piperun
```

---

## 📖 Configuration Reference

### Top-level block

```hcl
piperun {
  version = 2
}
```

### Variables

```hcl
variable "name" {
  type        = "string"
  default     = "world"
  description = "Who to greet"
}
```

Set via CLI: `piperun -var name=Alice`

### Stages

```hcl
stage "build" {
  depends_on = ["stage.lint", "stage.test"]
  script     = "go build ./..."

  pre_hook {
    script = "echo starting build..."
  }
  post_hook {
    script = "echo build finished!"
  }
}
```

### Conditional

```hcl
stage "ci_only" {
  if     = "env(\"CI\") == \"true\""
  script = "echo running in CI"
}
```

### Container

```hcl
stage "in_docker" {
  container {
    image = "node:20-alpine"
  }
  script = "node --version"
}
```

### Lifecycles

```hcl
stage "deploy" {
  lifecycle = ["deploy"]
  script    = "kubectl apply -f deploy.yaml"
}
```

Run only deploy stages: `piperun deploy`

### Allow / Block stages

```bash
piperun deploy +stage.extra_stage   # force-include a stage
piperun build ^stage.slow_test      # exclude a stage
```

---

## 🧪 Running Tests

```bash
cd piperun
go test ./...
```

---

## 🏗️ Project Structure

```
piperun/
├── cmd/piperun/         # CLI entrypoint
│   └── main.go
├── internal/
│   ├── dag/             # DAG builder & topological sort
│   ├── engine/          # Execution engine (parallel, hooks, containers)
│   ├── formatter/       # HCL formatter ("piperun fmt")
│   ├── logger/          # Coloured, per-stage logging
│   ├── parser/          # HCL parser → schema.Pipeline
│   └── schema/          # Data types (Pipeline, Stage, Variable, etc.)
├── examples/
│   ├── minimal/         # Hello-world pipeline
│   ├── full/            # Variables, deps, hooks, lifecycle
│   ├── conditional/     # if-based conditional stages
│   └── lifecycle/       # build vs deploy lifecycle demo
├── .goreleaser.yaml     # Cross-platform release config
├── Dockerfile           # Multi-stage Docker build
├── go.mod
├── LICENSE              # Apache 2.0
└── README.md
```

---

## 🗺️ Roadmap

- [ ] `module {}` block – reusable pipeline fragments from git/https/s3
- [ ] `for_each` – iterate stages over a collection
- [ ] `data {}` providers – env, git, prompt, terraform
- [ ] `macro {}` – inline reusable stage templates
- [ ] `import {}` – split pipelines across files
- [ ] Query engine (`--query` flag with HCL expressions)
- [ ] `locals {}` with full HCL expression evaluation
- [ ] Stage output capture (`this.status`, `this.stdout`)
- [ ] Retry / timeout per stage
- [ ] Remote execution (SSH, Kubernetes Jobs)
- [ ] Web UI dashboard

---

## 🤝 Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch (`git checkout -b feat/my-feature`)
3. Commit your changes (`git commit -am 'feat: add something'`)
4. Push to the branch (`git push origin feat/my-feature`)
5. Open a Pull Request

---

## 📜 License

[Apache License 2.0](LICENSE)

---

