Feature: A step cannot finish without a red run before its green run

  A test that nobody saw fail proves nothing. It can pass because the code is right, and it can pass
  because it asserts nothing at all. The two read the same from outside. So krewe holds the word done
  until it recorded two runs of the step's scenario: one that failed with at least one scenario in it,
  and a later one on the work as it stands.

  A step builds in two commits. The first commit holds the tests, and the run on it goes red. The
  second commit holds the code, and the run on it goes green. The red run is the record of the first.

  A run that failed with no scenario in it is not a red run. A name filter that matches nothing, and a
  command that cannot start, both report a failure that ran no test. A step could otherwise pass this
  gate with no line of it executed.

  The two refusals are two different moves, so each one says what to do next. A step nobody checked is
  told to run the check. A step whose tests nobody saw fail is told to write the tests first and watch
  them fail.

  Stopping a step is refused nothing. A step nobody will finish makes no promise about its tests.

  The project says whether any of this binds it. Both gates stand on a proof command, because that is
  what krewe runs to reach a verdict, and a project that never set one has nothing for krewe to run.
  So the take reads the project: a project that says how one scenario runs gets the gates and the
  restatement, and a project that says nothing works the way it did before them.

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

  # The whole feature in one scenario. The code passes its tests on the first run, so nothing was ever
  # seen to fail, and the word done is refused.
  Scenario: A step with no red run cannot finish
    Given the project's proof command is "go test ./features/... -run '{scenario}'"
    And the operator took step 1
    And krewe ran step 1's scenario, and it passed
    When the operator finishes step 1 with "shipped"
    Then the control plane refuses it as the wrong state
    And the refusal suggests "nobody saw step 1's tests fail"
    And the refusal suggests "krewe step check"
    And step 1 is still taken

  Scenario: A step whose tests were seen to fail first finishes
    Given the project's proof command is "go test ./features/... -run '{scenario}'"
    And the operator took step 1
    And krewe ran step 1's scenario, and it failed
    And krewe ran step 1's scenario, and it passed
    When the operator finishes step 1 with "shipped"
    Then step 1 is still done

  # Zero scenarios never passes, and zero scenarios never fails either. A run that executed nothing
  # says nothing about the tests.
  Scenario: A run that failed with no scenario in it is not a red run
    Given the project's proof command is "go test ./features/... -run '{scenario}'"
    And the operator took step 1
    And krewe ran step 1's scenario, and it failed with no scenario in it
    And krewe ran step 1's scenario, and it passed
    When the operator finishes step 1 with "shipped"
    Then the control plane refuses it as the wrong state
    And the refusal suggests "nobody saw step 1's tests fail"
    And step 1 is still taken

  # The other refusal, so the operator reads the move they are on rather than one sentence for two
  # states.
  Scenario: A step nobody checked is told that nothing ran
    Given the project's proof command is "go test ./features/... -run '{scenario}'"
    And the operator took step 1
    When the operator finishes step 1 with "shipped"
    Then the control plane refuses it as the wrong state
    And the refusal suggests "nothing checked step 1"
    And step 1 is still taken

  Scenario: Stopping a step needs no red run
    Given the project's proof command is "go test ./features/... -run '{scenario}'"
    And the operator took step 1
    When the operator stops step 1 with "the approach was wrong"
    Then step 1 is still stopped

  # A second attempt proves itself again. A red run carried over from the attempt that stopped would
  # let the next session write the code first.
  Scenario: A step taken again must see its tests fail again
    Given the project's proof command is "go test ./features/... -run '{scenario}'"
    And the operator took step 1
    And krewe ran step 1's scenario, and it failed
    And krewe ran step 1's scenario, and it passed
    And the operator stops step 1 with "the approach was wrong"
    And the operator takes step 1
    And krewe ran step 1's scenario, and it passed
    When the operator finishes step 1 with "shipped"
    Then the control plane refuses it as the wrong state
    And the refusal suggests "nobody saw step 1's tests fail"

  # The rule arrived after some work had started. A step whose row was written before it carries no
  # requirement, so the word done reads that row as a step the rule never bound and lets it close on
  # a check alone. Without this, every step a session was holding at the upgrade could never finish:
  # its code is written and its tests pass, so no run of it can go red any more.
  #
  # The row is the one a take under the rule never reached. Only a take writes the requirement, and
  # this suite holds its rows in memory and runs no migration, so a step the path wrote and nobody
  # took is the same row an upgrade leaves behind.
  Scenario: A step taken before the red run rule finishes without a red run
    Given the project's proof command is "go test ./features/... -run '{scenario}'"
    And the operator took step 1
    And the path holds step 2, whose row carries no red run requirement
    And krewe ran step 2's scenario, and it passed
    When the operator finishes step 2 with "shipped"
    Then step 2 is still done

  # The other half of the rule, read off the same path: step 1 was taken under it, so it is bound
  # whatever step 2 beside it is allowed to do.
  Scenario: A step taken under the rule is bound while a step beside it is not
    Given the project's proof command is "go test ./features/... -run '{scenario}'"
    And the operator took step 1
    And the path holds step 2, whose row carries no red run requirement
    And krewe ran step 1's scenario, and it passed
    When the operator finishes step 1 with "shipped"
    Then the control plane refuses it as the wrong state
    And the refusal suggests "nobody saw step 1's tests fail"
    And step 1 is still taken

  # The two projects this system was building when the gates arrived were both in this state: a path
  # the operator approved, a session holding a step, and nothing saying how one scenario runs. The
  # check could not pass, because krewe had no command to run, so the step could not close at all.
  # A project that asked for none of this gets the tool it had before: build the step, open the pull
  # request, say done.
  Scenario: A step in a project with no proof command finishes with no restatement and no check
    When the operator takes step 1
    Then the step text does not carry "Write no code. Change no file in the repository."
    And the step text carries "Deliver this step as one pull request."
    When the operator finishes step 1 with "shipped"
    Then step 1 is still done

  # The other half, read off a project that did ask. Setting a proof command is what turns the gates
  # on, and it turns all three on together: the restatement in the take text, the check, and the run
  # that must be seen to fail.
  Scenario: A step in a project that proves its steps is refused until something checked it
    Given the project's proof command is "go test ./features/... -run '{scenario}'"
    When the operator takes step 1
    Then the step text carries "Write no code. Change no file in the repository."
    When the operator finishes step 1 with "shipped"
    Then the control plane refuses it as the wrong state
    And the refusal suggests "nothing checked step 1"
    And step 1 is still taken

  # What the operator reads at the take, through the tool. The session that just started is building,
  # so a line saying it will restate the step and build nothing would send the operator away to wait
  # for a text that never arrives.
  Scenario: The take of a step in a project with no proof command says the session is building it
    Given the system listens on an address the tool can dial
    When the caller takes step "1.1"
    Then standard output carries "it will build the step and open a pull request"
    And standard output never says "it will restate the step and build nothing"
    And the command succeeds
