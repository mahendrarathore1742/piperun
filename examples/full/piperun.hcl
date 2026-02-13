# Demonstrates parallel stages, dependencies, variables, hooks, and lifecycles.
piperun {
  version = 2
}

variable "service" {
  type        = "string"
  default     = "myapp"
  description = "Name of the service to build"
}

stage "lint" {
  script = "echo Running lint for ${var.service}..."
}

stage "test" {
  script = "echo Running tests for ${var.service}..."
}

stage "build" {
  depends_on = ["stage.lint", "stage.test"]
  script     = "echo Building ${var.service}..."

  post_hook {
    script = "echo Build complete for ${var.service}!"
  }
}

stage "deploy" {
  depends_on = ["stage.build"]
  lifecycle  = ["deploy"]
  script     = "echo Deploying ${var.service} to production..."

  pre_hook {
    script = "echo Pre-deploy checks..."
  }
  post_hook {
    script = "echo Post-deploy notification sent."
  }
}
