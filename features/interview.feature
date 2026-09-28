Feature: The interview asks the operator before anything reads a repository

  Every design stage was written from a repository, and no stage asked the operator what they had in
  their head. So the stages came back long, and they described what the code already does rather than
  what the operator wants next.

  The interview is the first of the seven stages now. A session asks one question at a time, and
  every question carries its best guess, so the operator answers yes or corrects one word. It asks
  about who the product is for, the problem, what success looks like, the constraints, what is out of
  scope, and the decisions only the operator can take. It stops when it can predict the answers, and
  it says so.

  The page is a brief like every other stage: seven headings, in one order, inside one page. The
  answers go under them, so the constraints are assumptions, what success looks like is done when,
  and what is out of scope is not doing.

  A project that was already designed is not stranded. Migration 0081 marks its interview skipped, so
  the stages under it keep their words and the operator asks the questions later.

  Background:
    Given a running control plane
    And a workspace named "acme"
    And a project named "house-bills"

  # The order this feature exists for. A discovery is a reading of a repository, and the interview is
  # what it is read against, so the discovery waits.
  Scenario: The discovery waits for the interview
    When the operator writes the "discovery" design stage with the goal "three routes, and what each one draws"
    Then the control plane refuses it as the wrong state
    And the refusal suggests "interview"
    And the project holds no design stages

  Scenario: The interview is written into an empty project
    When the operator writes the "interview" design stage with the goal "they pay four bills, and two of them move"
    And the operator reads the project's design stages
    Then the "interview" design stage states the goal "they pay four bills, and two of them move"
    And the "interview" design stage is not approved
    And the "interview" design stage sits first

  Scenario: The discovery opens once the operator approves the interview
    Given the "interview" design stage is written and approved
    When the operator writes the "discovery" design stage with the goal "three routes, and what each one draws"
    And the operator reads the project's design stages
    Then the "discovery" design stage states the goal "three routes, and what each one draws"

  # The interview is written from a conversation rather than from files, so it is the stage most
  # likely to come back as a transcript. It is held to the shape every stage is held to, and the
  # refusal names the heading that is missing.
  Scenario: An interview that leaves a heading out is refused, and the refusal names it
    When the operator writes the "interview" design stage as:
      """
      # The interview

      ## Goal

      they pay four bills a month, and two of them move.

      ## Direction

      one screen for the week ahead, on a phone, on the day a bill lands.

      ## Assumptions

      - one evening a week, and no server they pay for. Correct me or I proceed.

      ## Done when

      - they know what is due this week without opening a bank app.

      ## Not doing

      - payments, because the bank does them.

      ## Open questions

      - what happens to a bill after somebody pays it?
      """
    Then the control plane refuses it as invalid
    And the refusal suggests "Decisions for the operator"
    And the project holds no design stages

  # What a session reads before it asks anybody anything. The brief is prose and nothing else: no
  # gate reads it, so what it fails to say is a thing the interview will not do.
  Scenario Outline: The interview skill says how the questions are asked
    When the operator reads the interview skill
    Then the interview skill says "<said>"

    Examples:
      | said                       |
      | one question at a time     |
      | best guess                 |
      | ten questions              |
      | predict the answers        |
      | in the operator's own words|
      | explicit yes               |
      | Only the operator approves |

  # The page is a brief, and the brief says where each answer goes. A session told only to ask the
  # questions writes a transcript under the first heading.
  Scenario Outline: The interview skill says where each answer goes in the brief
    When the operator reads the interview skill
    Then the interview skill says "<said>"

    Examples:
      | said                       |
      | Goal                       |
      | Direction                  |
      | Assumptions                |
      | Decisions for the operator |
      | Done when                  |
      | Not doing                  |
      | Open questions             |
      | the constraints            |
      | what is out of scope       |
      | no transcript              |

  # The stage after it reads it first. A later stage that disagrees with the interview says so as a
  # decision, because the operator answered those questions and nothing else did.
  Scenario: The interview skill says every later stage reads it first
    When the operator reads the interview skill
    Then the interview skill says "stage after it reads the interview first"
    And the interview skill says "Never contradict the interview quietly"
