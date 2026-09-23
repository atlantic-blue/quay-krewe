Feature: The design command walks the design stages in order

  The six stages sat on the project, and nothing walked them. The operator read the listing, picked a
  stage, and remembered which one came next. The design command asked its five questions and
  dispatched one session to write the whole design, so the data of a project and its screens were
  designed in one breath again.

  The command reads `krewe stage show` now. It starts at the first stage without the operator's word,
  and it dispatches one session for that stage alone. The order stays in the control plane, because a
  command file with an order of its own is a second copy of the rule.

  So the data model comes after the screens. A project that carries only its discovery starts at the
  stories. A project with four approved stages starts at the data model. A project with nothing on it
  starts at the discovery, and `/krewe:discover` is the command that writes that one.

  The design skill is the other half of the step. A session reads all six approved stages before it
  writes the design, the features, the contracts and the paths. Each contract says which stage it
  came from, so a reader goes back to the words the operator approved.

  Background:
    Given a running control plane
    And the system listens on an address the tool can dial
    And a workspace named "acme"
    And a project named "house-bills"
    And the operator's agent reads its commands from a directory of its own
    And the slash commands are installed

  # The one this step exists for. The stage comes out of the command the file names, run against a
  # real project, so a file that stops reading the listing fails here.
  Scenario: The design command on a project with no stories starts at stories
    Given the "discovery" design stage is written and approved
    When the operator reads where the design starts, the way the design command reads it
    Then the command succeeds
    And the design starts at "stories"
    And the design starts at no other stage

  # One step past the printed line. The stage the listing offered is a stage a session may write now,
  # and the operator ends with it on the project.
  Scenario: The stage the design command starts at is one a session may write now
    Given the "discovery" design stage is written and approved
    When the operator reads where the design starts, the way the design command reads it
    Then the design starts at "stories"
    And the operator writes the "stories" design stage as "I want to see what is due"
    And the "stories" design stage reads "I want to see what is due"

  # The data comes last of the five the design command carries. Four stages hold the word here, and
  # the screens are among them, so the data model opens only now.
  Scenario: The design command reaches the data model after the operator approves the mockups
    Given the "discovery" design stage is written and approved
    And the "stories" design stage is written and approved
    And the "design_system" design stage is written and approved
    And the "mockups" design stage is written and approved
    When the operator reads where the design starts, the way the design command reads it
    Then the command succeeds
    And the design starts at "data_model"
    And the design starts at no other stage

  # The screens themselves, in the middle of the six. The stories and the design system hold the
  # word, and the data model is still shut.
  Scenario: The design command starts at the mockups while the screens hold no word
    Given the "discovery" design stage is written and approved
    And the "stories" design stage is written and approved
    And the "design_system" design stage is written and approved
    When the operator reads where the design starts, the way the design command reads it
    Then the command succeeds
    And the design starts at "mockups"
    And the design starts at no other stage

  # A written stage is not an agreed stage. The design stays on it, because the control plane refuses
  # the stage after a stage that nobody read.
  Scenario: The design command stays on a stage that nobody approved
    Given the operator writes the "discovery" design stage as "four bills, and two of them move"
    When the operator reads where the design starts, the way the design command reads it
    Then the command succeeds
    And the design starts at "discovery"
    And the reading offers that stage for approval

  # Nothing on the project at all. Discovery is the first of the six, the discover command writes it,
  # and the design command names that command rather than asking for it here.
  Scenario: The design command starts at discovery on a project with no stages
    When the operator reads where the design starts, the way the design command reads it
    Then the command succeeds
    And the design starts at "discovery"
    And the installed command "design" names "/krewe:discover"

  # Six words on six stages. The stages are done, so the design of the whole project is the move that
  # is left.
  Scenario: The design command designs the project once every stage holds the word
    Given every design stage is written and approved
    When the operator reads where the design starts, the way the design command reads it
    Then the command succeeds
    And the reading names no stage of the six
    And the reading says the project is ready to build

  # The file holds no order. It reads the listing, and the listing names the stage this run is about,
  # so a file that dispatched the six in a row of its own would design the data before the screens.
  Scenario: The design command reads the listing and dispatches for one stage
    When the operator reads the design command
    Then the installed command "design" names "krewe stage show <workspace>/<project>"
    And the installed command "design" names "first stage without the operator's word"
    And the design command reads the stage listing before it runs anything else
    And the design command dispatches a session for the stage it read, and for no stage of its own

  # Approval is the operator's word, on one stage and on the text it holds now. So the yes sits beside
  # the command that records it.
  Scenario: The design command asks for a yes before it approves a stage
    When the operator reads the design command
    Then the installed command "design" names "krewe stage approve"
    And the installed command "design" asks for a yes where it runs "krewe stage approve"
    And the installed command "design" says a no leaves the design unapproved

  # What a session reads before it writes a design. The brief is prose and nothing else: no gate
  # reads it, so what it fails to say is a thing the design will not hold.
  Scenario Outline: The design skill reads every approved stage before it writes a design
    When the operator reads the design skill
    Then the design skill says "<said>"

    Examples:
      | said                                                |
      | krewe stage show                                    |
      | Read all six stages before you write anything       |
      | stories                                             |
      | design_system                                       |
      | mockups                                             |
      | data_model                                          |
      | architecture                                        |
      | Name the stage each contract came from              |
