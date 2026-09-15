Feature: A session runs on the container runtime the operator chose

  Docker is not the only way to run a container. containerd is the runtime the Docker daemon runs its
  own containers with, and it is the runtime Kubernetes moved to when it dropped the Docker shim in
  version 1.24. An operator who runs containerd directly runs sessions from the same images with no
  daemon above them. So the system takes the name of a runtime, and it builds the backend for that
  name.

  Docker stays what a system gets when nobody chose. A name the system does not have stops it
  starting, because a system asked for one runtime and quietly running another looks exactly like a
  system that was asked for the one it is running.

  These scenarios read the configuration. They start no container. That a containerd container really
  runs a session is proved by the provider conformance suite, which both backends are held to, and
  that suite needs a real runtime.

  Scenario: Nothing chosen runs a session on Docker
    Given the system is told to run sessions on ""
    Then the system runs sessions on "docker"
    And a session gets a container of its own on the Docker daemon

  Scenario: An operator who runs containerd says so
    Given the system is told to run sessions on "containerd"
    Then the system runs sessions on "containerd"
    And a session gets a container of its own on containerd

  Scenario: The runtime with no isolation is still there
    Given the system is told to run sessions on "local"
    Then the system runs sessions on "local"

  Scenario: A runtime the system does not have stops it starting
    Given the system is told to run sessions on "podman"
    Then the system refuses the runtime
    And the refusal about the runtime names "podman"
