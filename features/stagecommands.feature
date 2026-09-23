Feature: The operator writes and approves the design stages from the command line

  The six stages a project is designed in were reachable only through the system's own calls. An
  operator with a discovery document on their machine had no way to put it on the project, and
  nothing said where a project was up to.

  Three words do it. `krewe stage show` lists the six in order with the state of each one, so a
  person reads empty, written, approved or skipped at a glance, and the line under the listing names
  the one move the project has. `krewe stage set` writes one stage from a file, and the artifact
  beside it from a second file. `krewe stage approve` records the operator's word on the stage as it
  stands.

  The tool holds no rule about the order. It sends one stage and prints what came back, so the
  refusal an operator reads is the system's own words.

  Approving is the operator's, and a session is refused the call. A session writes a stage, the write
  clears the approval, and somebody still has to read it. A session that could approve its own stage
  would agree with itself six times and then build from it.

  Background:
    Given a running control plane
    And the system listens on an address the tool can dial
    And a workspace named "acme"
    And a project named "house-bills"

  Scenario: The listing names all six stages and the state of each one
    When the caller types "stage show acme/house-bills" through the tool
    Then the command succeeds
    And standard output says "discovery"
    And standard output says "architecture"
    And standard output says "empty"

  # The one move the project has. Six rows of empty say nothing about which of them a person may
  # write, and the order is the rule this whole feature holds.
  Scenario: The listing says which stage to write next
    When the caller types "stage show acme/house-bills" through the tool
    Then the command succeeds
    And standard output says "krewe stage set acme/house-bills discovery"

  Scenario: A stage is written from a file and then approved
    Given a stage file saying "They pay four bills, and two move."
    When the caller writes the "discovery" design stage from that file
    Then the command succeeds
    And the caller approves the "discovery" design stage
    And the command succeeds
    And the operator reads the project's design stages
    And the "discovery" design stage reads "They pay four bills, and two move."
    And the "discovery" design stage is approved

  # Written is not agreed, so the listing offers the reading rather than the writing again.
  Scenario: A written stage is offered for approval
    Given a stage file saying "They pay four bills, and two move."
    And the caller writes the "discovery" design stage from that file
    When the caller types "stage show acme/house-bills" through the tool
    Then the command succeeds
    And standard output says "written"
    And standard output says "krewe stage approve acme/house-bills discovery"

  # The refusal the whole feature exists for, read as an operator reads it. It names the stage to go
  # and approve, and it says nothing was written, because somebody who thinks half a write landed
  # goes looking for it.
  Scenario: A stage written out of order is refused and nothing is written
    Given a stage file saying:
      """
      one table for each bill

      ```mermaid
      erDiagram
        BILL ||--o{ PAYMENT : has
      ```
      """
    When the caller writes the "data_model" design stage from that file
    Then the command fails
    And standard error says "discovery"
    And standard error says "nothing was written"
    And the project holds no design stages

  # The gate is only real while nothing inside a sandbox can pass it. A session writes a stage, and
  # that write clears the approval, so a session that could then approve it would be agreeing with
  # itself.
  Scenario: A session cannot approve a design stage
    Given the operator writes the "discovery" design stage as "what we asked"
    When the driver asks to approve the "discovery" design stage
    Then the driver is refused, told the call is the operator's to make
    And the operator reads the project's design stages
    And the "discovery" design stage is not approved
