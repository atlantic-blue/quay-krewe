Feature: A project is designed in stages before it is built

  A project used to start from one design document covering everything at once, so what a person sees
  and what the data looks like were written in the same breath. The data model usually won first,
  because it is the easiest part to write, and the screens were then fitted to it.

  A project now carries six stages in an order: discovery, stories, design_system, mockups,
  data_model, architecture. A stage cannot be written while a stage before it carries no approval, so
  the data model can never be written before the stories are agreed.

  Approval is a statement about one text. A write to a stage takes the word off that stage, and off
  every stage after it, because those were agreed under the text that just changed. The words stay:
  only the word on them is gone. The stage that was written keeps the version the word was given to,
  so a reader can tell it from a stage nobody ever agreed to.

  Two of the six describe a structure. The data model and the architecture are refused until the
  body holds a diagram, written as a fenced block marked mermaid, so the operator approves a picture
  rather than a paragraph. The other four are prose and carry whatever they carry.

  A project that has written no stage holds none, which is how every project made before this
  existed reads, and it keeps working exactly as it did.

  Background:
    Given a running control plane
    And a workspace named "acme"
    And a project named "house-bills"

  Scenario: A project nobody staged holds no design stages
    When the operator reads the project's design stages
    Then the project holds no design stages

  Scenario: The first stage is written into an empty project
    When the operator writes the "discovery" design stage as "They pay four bills, and two move."
    And the operator reads the project's design stages
    Then the "discovery" design stage reads "They pay four bills, and two move."
    And the "discovery" design stage is not approved

  # The refusal this whole feature exists for. It names the stage to go and approve, because an
  # operator told only no has to work out which of five stages they are missing.
  #
  # The body carries a diagram because the data model is one of the two stages that must, and a body
  # without one is refused before the order is read at all.
  Scenario: A stage is refused while the stage before it is not approved
    When the operator writes the "data_model" design stage as:
      """
      one table for each bill

      ```mermaid
      erDiagram
        BILL ||--o{ PAYMENT : has
      ```
      """
    Then the control plane refuses it as the wrong state
    And the refusal suggests "discovery"
    And the project holds no design stages

  Scenario: A stage goes in once the stage before it is approved
    Given the "discovery" design stage is written and approved
    When the operator writes the "stories" design stage as "I want to see what is due"
    And the operator reads the project's design stages
    Then the "stories" design stage reads "I want to see what is due"

  # Written is not agreed. A stage sitting there unread is no better than a stage nobody wrote, which
  # is the half of the rule a check for the row alone would miss.
  Scenario: A stage written and never approved still refuses the stage after it
    Given the operator writes the "discovery" design stage as "what we asked"
    When the operator writes the "stories" design stage as "I want to see what is due"
    Then the control plane refuses it as the wrong state
    And the refusal suggests "discovery"

  # The first stage without a word on it, not the nearest one: the operator's next move is at the top
  # of the list, and a refusal naming the mockups sends them to a stage they cannot write either.
  Scenario: The refusal names the first stage without approval
    Given the "discovery" design stage is written and approved
    And the operator writes the "stories" design stage as "I want to see what is due"
    When the operator writes the "design_system" design stage as "one accent colour"
    Then the control plane refuses it as the wrong state
    And the refusal suggests "stories"

  Scenario: The six stages are written and approved in order
    Given every design stage is written and approved
    When the operator reads the project's design stages
    Then the project holds 6 design stages, in order
    And every design stage is approved

  # The text moved under the word, so the word stops standing. The version it was given to stays on
  # the stage, because a stage that changed after it was agreed is a different thing from a stage
  # nobody ever agreed to, and the page that lists the six says which of the two this is.
  Scenario: Rewriting a stage takes its own approval away
    Given the "discovery" design stage is written and approved
    When the operator writes the "discovery" design stage as "what we asked, the second time"
    And the operator reads the project's design stages
    Then the "discovery" design stage is not approved
    And the "discovery" design stage reads approved at version 1, holding version 2

  # Every stage after the one that changed was agreed under a text that has just moved, so the word
  # on each of them is gone and the write says which ones by name.
  Scenario: Rewriting a stage takes the approval off every stage after it
    Given every design stage is written and approved
    When the operator writes the "stories" design stage as "the stories, rethought"
    Then the write says the approval went from "mockups"
    And the write says the approval went from "architecture"
    And the operator reads the project's design stages
    And the "discovery" design stage is approved
    And the "mockups" design stage is not approved
    And the "architecture" design stage is not approved

  Scenario: A stage nobody wrote cannot be approved
    When the operator approves the "discovery" design stage
    Then the control plane refuses it as the wrong state
    And the refusal suggests "krewe stage set"

  Scenario: A name outside the six is not a design stage
    When the operator writes the "wireframes" design stage as "the wireframes"
    Then the control plane refuses it as invalid
    And the refusal suggests "design_system"

  # The artifact is the structured document a stage carries beside its prose, such as the flows the
  # mockups are played from. It is kept as json, because a reader opens it as json.
  Scenario: A stage carries a json artifact beside its prose
    When the operator writes the "discovery" design stage with the artifact:
      """
      {"asked": ["when does it move"]}
      """
    And the operator reads the project's design stages
    Then the "discovery" design stage carries an artifact meaning:
      """
      {"asked":["when does it move"]}
      """

  Scenario: An artifact that is not json is refused
    When the operator writes the "discovery" design stage with the artifact:
      """
      the flows are over there
      """
    Then the control plane refuses it as invalid
    And the project holds no design stages

  # A data model and an architecture each describe a structure. Prose about a structure is read a
  # different way by every reader, so these two stages carry a picture of it. The write is refused
  # until the body holds a fenced block marked mermaid, and the operator then approves a diagram
  # rather than a paragraph.
  Scenario: An architecture stage with no diagram is refused
    Given the first 5 design stages are written and approved
    When the operator writes the "architecture" design stage as "the control plane holds the store, and a session talks to the control plane"
    Then the control plane refuses it as invalid
    And the refusal suggests "mermaid"
    And the project holds no "architecture" design stage

  Scenario: An architecture stage with a diagram goes in
    Given the first 5 design stages are written and approved
    When the operator writes the "architecture" design stage as:
      """
      the control plane holds the store

      ```mermaid
      flowchart LR
        session --> controlplane --> store
      ```
      """
    And the operator reads the project's design stages
    Then the "architecture" design stage carries a diagram
    And the "architecture" design stage is not approved

  Scenario: A data model with no diagram is refused
    Given the first 4 design stages are written and approved
    When the operator writes the "data_model" design stage as "one table for each bill, and one row for each payment"
    Then the control plane refuses it as invalid
    And the refusal suggests "data_model"
    And the project holds no "data_model" design stage

  # The rule is about the two stages that carry a structure. The other four are written as prose, and
  # a rule over all six would refuse a discovery nobody can draw.
  Scenario: A stage that is not the data model or the architecture needs no diagram
    Given the "discovery" design stage is written and approved
    When the operator writes the "stories" design stage as "I want to see what is due"
    And the operator reads the project's design stages
    Then the "stories" design stage reads "I want to see what is due"
