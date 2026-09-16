Feature: A session is isolated by the runtime the operator chose

  A session runs somewhere. Which runtime that is decides what a session costs on the machine, what a
  session can reach, and what the operator must install. The system knows each runtime by one word,
  and the operator sets that word in the system's configuration.

  Docker is the default, and it gives each session a container of its own. Apple container gives each
  session a container too, in one light virtual machine of its own, on a macOS machine and with no
  Docker Desktop. containerd gives each session a container from the same images with no Docker daemon
  above it, which is the runtime Kubernetes uses. The host backend runs a session on the machine itself
  with no isolation, and it is a stopgap.

  A word the system does not know is refused. A system that fell back to the default would isolate
  every session in something the operator did not choose, and would then report that choice as theirs.

  These scenarios read the choice. What a backend does against its own runtime is the contract in
  internal/sandbox/sandboxtest: Docker runs it in continuous integration, and Apple container and
  containerd run the same cases on a machine that holds their tool.

  Scenario: A system that names no runtime puts a session in a Docker container
    Given the system names no runtime for a session
    When the system builds the backend a session runs in
    Then it reports the runtime "docker"
    And it builds the Docker backend

  Scenario: An operator chooses Apple container
    Given the system is configured to isolate a session with "apple"
    When the system builds the backend a session runs in
    Then it reports the runtime "apple"
    And it builds the Apple container backend

  Scenario: An operator runs containerd and no Docker daemon
    Given the system is configured to isolate a session with "containerd"
    When the system builds the backend a session runs in
    Then it reports the runtime "containerd"
    And it builds the containerd backend

  Scenario: An operator runs a session on the machine itself
    Given the system is configured to isolate a session with "local"
    When the system builds the backend a session runs in
    Then it reports the runtime "local"
    And it builds the host backend

  Scenario: A runtime the system does not know is refused
    Given the system is configured to isolate a session with "podman"
    When the system builds the backend a session runs in
    Then it refuses to build a backend
    And the refusal names the word "podman"
    And it reports no runtime at all
