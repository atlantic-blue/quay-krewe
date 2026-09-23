Feature: After its red run, a building session cannot change a test

  A session writes the tests, watches them fail, and then writes the code that makes them pass. The
  shortest way from red to green is to weaken the test, and a session that takes it is not being
  dishonest: from the inside, a failing test looks exactly like a wrong one. The suite is the only
  thing holding the requirement, so a build that edits it proves nothing.

  The gate that refuses the write already ships. It was on only for a session carrying the variable
  KREWE_BUILDING, and nothing ever set that variable, so it was off in every session that ever ran.

  The moment it comes on is the whole of this step. It cannot come on at the take, because the same
  session writes the tests first and a gate that was on then would refuse the session for doing what
  the step asked. So it comes on when krewe records the run that saw those tests fail, and not before.

  A container that is already running cannot be handed a new variable, and the red run lands in the
  middle of a session's life. So the system writes a mark into the session's own .krewe directory, and
  the gate reads it on every tool call. A session may read that mark. It may not write it, and it may
  not take it away: a boundary a session can lift is advice with extra steps.

  Background:
    Given a running control plane
    And a workspace named "acme"
    And a project named "house-bills"
    And a step taken, restated and approved, naming the scenario "a project carries a brief"
    And the project's proof command is "go test ./features/... -run '{scenario}'"

  # The whole step in one scenario. The run goes red, and the session that watched it go red is
  # refused the edit that would make it green.
  Scenario: A session cannot change a test after its red run
    Given the run answers "1 scenarios (0 passed, 1 failed)\nthe brief read back empty" and exits 1
    When the operator checks step 1
    Then the session holding the step is under the test gate
    And the test gate refuses that session a write to "features/brief_test.go"
    And the refusal says to answer that the test is wrong instead

  # The other half, and the reason the gate cannot simply be on from the take. This session has to
  # write the tests before anything can watch them fail.
  Scenario: Before its red run the session writes its tests
    Then the session holding the step is not under the test gate
    And the test gate allows that session a write to "features/brief_test.go"

  # A run that passed the first time saw nothing fail. It is not a red run, so it turns nothing on,
  # and the session goes back to writing the tests it never had.
  Scenario: A run that passed the first time turns nothing on
    Given the run answers "1 scenarios (1 passed)" and exits 0
    When the operator checks step 1
    Then the session holding the step is not under the test gate
    And the test gate allows that session a write to "features/brief_test.go"

  # A run that failed with no scenario in it executed nothing, so it says nothing about the tests.
  # This is the same reading the word done makes, and the two must agree.
  Scenario: A failure that ran no scenario turns nothing on
    Given the run answers "0 scenarios (0 passed)" and exits 1
    When the operator checks step 1
    Then the session holding the step is not under the test gate

  Scenario: A session cannot take the mark away
    Given the run answers "1 scenarios (0 passed, 1 failed)" and exits 1
    When the operator checks step 1
    Then the test gate refuses that session the command "rm .krewe/building"
    And the test gate refuses that session the command "rm -rf .krewe"
    And the test gate allows that session the command "cat .krewe/building"

  # The mark sits in one session's own directory, so it says nothing about any other session. The
  # session that writes the tests for the next step holds no step of its own and is refused nothing.
  Scenario: The gate is on for the session holding the step and for no other
    Given the run answers "1 scenarios (0 passed, 1 failed)" and exits 1
    When the operator checks step 1
    Then the session holding the step is under the test gate
    And a session holding no step is not under the test gate

  # The mark is written again on every exec, out of the record the store holds, rather than only at
  # the moment of the run. So a session that lost it, to a container replaced or to a hand reaching
  # into the directory, comes back under the gate at its next exec.
  Scenario: The gate is put back at the next exec
    Given the run answers "1 scenarios (0 passed, 1 failed)" and exits 1
    And the operator checks step 1
    And the mark is taken off the session from outside
    When the operator dispatches "carry on" to the same session
    Then the session holding the step is under the test gate
    And the test gate refuses that session a write to "features/brief_test.go"

  # And it goes away when the work does. A step nobody holds any more leaves no session under a gate
  # that nothing will ever lift.
  Scenario: The gate comes off when the step is over
    Given the run answers "1 scenarios (0 passed, 1 failed)" and exits 1
    And the operator checks step 1
    And the session holding the step is under the test gate
    When the operator stops step 1 with "the approach was wrong"
    And the operator dispatches "carry on" to the same session
    Then the session holding the step is not under the test gate
    And the test gate allows that session a write to "features/brief_test.go"
