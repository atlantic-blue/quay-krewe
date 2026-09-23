Feature: A staged project refuses a step until every stage is approved

  The six design stages say what a person will see before anything says what the data looks like.
  They mean nothing while a step can be taken over a mockup nobody read, so the take reads them.

  A project that holds at least one stage is designed in stages, and no step of it is taken until
  every one of the six carries the operator's word. The refusal names the first stage without it,
  because that is the operator's next move, and it says whether that stage is written yet.

  A project that holds no stage is refused nothing. That is every project made before the stages
  existed, and each one stands on an approved design document instead, so the work in flight in them
  keeps moving exactly as it did.

  The design document is still read first. A project whose design nobody approved gets that refusal,
  whatever its stages say.

  Background:
    Given a running control plane
    And a workspace named "acme"
    And a project named "house-bills"
    And the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      """

  # The state an operator is in the moment before they finish: five stages agreed and the last one
  # written and unread. Nothing is built until they read it.
  Scenario: A staged project with five approved stages refuses a step
    Given the first 5 design stages are written and approved
    And the operator writes the "architecture" design stage as "one table for each bill"
    When the operator takes step 1
    Then the control plane refuses it as the wrong state
    And the refusal suggests "architecture"
    And the refusal suggests "krewe stage approve"
    And step 1 is still ready

  # The first stage without the word, not the nearest one. An operator sent to the data model is sent
  # to a stage they cannot write either.
  Scenario: The take names the first stage without approval
    Given the "discovery" design stage is written and approved
    And the operator writes the "stories" design stage as "I want to see what is due"
    When the operator takes step 1
    Then the control plane refuses it as the wrong state
    And the refusal suggests "stories"

  # A stage nobody wrote and a stage nobody read are two different moves, so the refusal says which
  # one this is rather than asking for a word on a text that is not there.
  Scenario: A stage nobody wrote is refused as a stage to write
    Given the "discovery" design stage is written and approved
    When the operator takes step 1
    Then the control plane refuses it as the wrong state
    And the refusal suggests "krewe stage set"

  Scenario: A staged project builds once every stage is approved
    Given every design stage is written and approved
    When the operator takes step 1
    Then step 1 is held by that session

  # The half that costs something if it breaks. Three projects are being built this way today.
  Scenario: A project that holds no design stages takes a step as it always did
    Given the project holds no design stages
    When the operator takes step 1
    Then step 1 is held by that session
