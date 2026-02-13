# Demonstrates lifecycle-based execution
# Run with: piperun build   OR   piperun deploy
piperun {
  version = 2
}

stage "compile" {
  lifecycle = ["build", "deploy"]
  script    = "echo Compiling..."
}

stage "unit_test" {
  lifecycle  = ["build"]
  depends_on = ["stage.compile"]
  script     = "echo Running unit tests..."
}

stage "push_image" {
  lifecycle  = ["deploy"]
  depends_on = ["stage.compile"]
  script     = "echo Pushing Docker image..."
}

stage "notify" {
  lifecycle  = ["all"]
  depends_on = ["stage.compile"]
  script     = "echo Sending notification..."
}
