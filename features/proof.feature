Feature: A scenario name reaches the proof command as one word

  A proof command carries the token {scenario}, and krewe puts the step's scenario name there before it
  runs the command with a shell. A name is prose. It holds spaces, quotes, backticks and dollar signs,
  and a shell reads every one of them as code. A name that reached the shell as plain text ended one
  run with a syntax error, so the run executed 0 scenarios and the verdict said nothing about the code.

  So krewe quotes the name itself. It hands the shell one word, whatever the name holds, and the name
  arrives at the runner exactly as the feature file writes it.

  The command author writes no quote around the token. Two pairs of quotes split the name again, so a
  command that puts the token inside a quoted span is refused, and the refusal shows the same command
  with the token outside the quotes. A refused command writes nothing, and the command the project
  already had stays where it is.

  Background:
    Given a running control plane
    And a workspace named "acme"
    And a project named "house-bills"

  # CHECK-3. The name is data and never code. The proof is a real shell: the scenario reads the command
  # krewe handed the sandbox, runs it, and reads the name back. The format prints one pair of brackets
  # around each argument, so a name the shell split arrives in several pairs and fails here.
  Scenario: A scenario name with quotes and backticks reaches the proof command as one word
    Given a step taken, restated and approved, naming the scenario:
      """
      A key of `../../etc/passwd` isn't "outside" it, and costs $0
      """
    And the project's proof command is "printf [%s] {scenario}"
    And the run reports one scenario
    When the operator checks step 1
    Then a shell given what the run was given prints the scenario name as one word
    And step 1 reads back as passing
    And step 1 ran 1 scenario

  # CHECK-4. The refusal names the command to type, because a person who reads it is about to type the
  # command again. What was quoted around the token keeps its quotes where the shell reads it.
  Scenario: A proof command that quotes the scenario token is refused, and shows how to write it
    Given the system listens on an address the tool can dial
    When the caller sets the project's proof command to "go test ./features/... -run 'TestFeatures/{scenario}'"
    Then the command fails
    And standard error says "krewe quotes the scenario name"
    And standard error says "-run TestFeatures/{scenario}"
    And the project proves nothing

  Scenario: A proof command that quotes the scenario token in double quotes is refused
    When the operator sets the project's proof command to:
      """
      go test ./features/ -count=1 -v -run "TestFeatures/{scenario}$"
      """
    Then the control plane refuses it as invalid
    And the refusal suggests "-run TestFeatures/{scenario}'$'"
    And the project proves nothing

  Scenario: A proof command with the scenario token outside the quotes is accepted
    When the operator sets the project's proof command to "go test ./features/ -count=1 -v -run TestFeatures/{scenario}'$'"
    Then the project proves one scenario with "go test ./features/ -count=1 -v -run TestFeatures/{scenario}'$'"

  Scenario: A refused command leaves the command the project already had
    Given the project's proof command is "make one {scenario}"
    When the operator sets the project's proof command to "go test -run '{scenario}'"
    Then the control plane refuses it as invalid
    And the project proves one scenario with "make one {scenario}"
