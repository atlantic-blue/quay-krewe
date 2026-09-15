Feature: A macOS guest is scarce, so a session queues for one

  A session runs in a Linux container, so it can lint an iOS application, typecheck it and run its
  tests. It can never build it, because Xcode does not run on Linux. The pipeline is Linux too. So a
  person proves a native build by hand on one machine, after the work merges. On 15 September 2026 a
  crash that stopped an application from starting went through eleven green pull requests that way.

  A session can ask for a macOS guest instead of a container. Which runtime a session gets is
  sandboxbackends.feature. This one is about what makes that runtime different from every other one:
  the operator cannot have as many as they want.

  The number two is Apple's, not a choice. Section 2B(iii) of the macOS Tahoe 26 Software License
  Agreement permits two instances of macOS in virtual environments. The computer must be an Apple one
  the operator owns, and the purpose must be software development or testing during it. The
  Virtualization framework refuses the third guest as well. So a third session waits for a guest,
  rather than taking one that cannot start.

  These scenarios drive a stand in for tart, so they prove the queue and they prove nothing about
  Apple's framework. The contract in internal/sandbox/sandboxtest is what a real guest is held to.

  Scenario: Two macOS guests run at once, and the third session waits for one
    Given a host running the macOS sandbox
    And 2 sessions already hold a guest
    When a third session asks for one
    Then it waits, and the refusal names the number this host may run

  Scenario: A session that already holds a guest is given the same one
    Given a host running the macOS sandbox
    And a session already holds a guest
    When that session asks for one again
    Then it is given the guest it already has
    And the host still runs 1 guest
