# Demonstrates conditional execution
piperun {
  version = 2
}

stage "always" {
  script = "echo This always runs"
}

stage "ci_only" {
  if     = "env(\"CI\") == \"true\""
  script = "echo This only runs in CI"
}
