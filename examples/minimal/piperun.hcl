# A minimal piperun pipeline
piperun {
  version = 2
}

stage "hello" {
  script = "echo Hello from piperun!"
}
