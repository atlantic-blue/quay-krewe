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
    And the operator took step 1

  # The whole feature in one scenario. The code passes its tests on the first run, so nothing was ever
  # seen to fail, and the word done is refused.
  Scenario: A step with no red run cannot finish
    Given krewe ran step 1's scenario, and it passed
    When the operator finishes step 1 with "shipped"
    Then the control plane refuses it as the wrong state
    And the refusal suggests "nobody saw step 1's tests fail"
    And the refusal suggests "krewe step check"
    And step 1 is still taken

  Scenario: A step whose tests were seen to fail first finishes
    Given krewe ran step 1's scenario, and it failed
    And krewe ran step 1's scenario, and it passed
    When the operator finishes step 1 with "shipped"
    Then step 1 is still done

  # Zero scenarios never passes, and zero scenarios never fails either. A run that executed nothing
  # says nothing about the tests.
  Scenario: A run that failed with no scenario in it is not a red run
    Given krewe ran step 1's scenario, and it failed with no scenario in it
    And krewe ran step 1's scenario, and it passed
    When the operator finishes step 1 with "shipped"
    Then the control plane refuses it as the wrong state
    And the refusal suggests "nobody saw step 1's tests fail"
    And step 1 is still taken

  # The other refusal, so the operator reads the move they are on rather than one sentence for two
  # states.
  Scenario: A step nobody checked is told that nothing ran
    When the operator finishes step 1 with "shipped"
    Then the control plane refuses it as the wrong state
    And the refusal suggests "nothing checked step 1"
    And step 1 is still taken

  Scenario: Stopping a step needs no red run
    When the operator stops step 1 with "the approach was wrong"
    Then step 1 is still stopped

  # A second attempt proves itself again. A red run carried over from the attempt that stopped would
  # let the next session write the code first.
  Scenario: A step taken again must see its tests fail again
    Given krewe ran step 1's scenario, and it failed
    And krewe ran step 1's scenario, and it passed
    And the operator stops step 1 with "the approach was wrong"
    And the operator takes step 1
    And krewe ran step 1's scenario, and it passed
    When the operator finishes step 1 with "shipped"
    Then the control plane refuses it as the wrong state
    And the refusal suggests "nobody saw step 1's tests fail"
