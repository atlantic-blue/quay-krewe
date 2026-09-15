Feature: A session can be given a macOS virtual machine

  A session runs in a Linux container, so it can lint an iOS application, typecheck it and run its
  tests, and it can never build it. Xcode does not run on Linux. The pipeline is Linux too, so a
  native build is only ever proved by hand on one machine, after the work has already merged. On 15
  September 2026 a crash that stopped an application from starting shipped through eleven green pull
  requests that way.

  So a session can ask for a macOS guest instead of a container. The container stays the default and
  nothing else changes: a session that does not ask for one is not affected.

  The number two in these scenarios is Apple's, not a choice. Section 2B(iii) of the macOS Tahoe 26
  Software License Agreement permits two instances of macOS in virtual environments on one
  Apple computer you own, for software development and for testing during it. The Virtualization
  framework refuses the third. A guest is therefore scarce in a way a container never was, so a
  session queues for one rather than being handed one that cannot start.

  Scenario: The system is told to isolate a session in a macOS virtual machine
    Given a system configured with the sandbox kind "macos"
    Then a session is isolated in a macOS virtual machine

  Scenario: A container is still what a session gets by default
    Given a system configured with no sandbox kind
    Then a session is isolated in a container

  # A typo must not quietly become the default. An operator who asked for a macOS guest and got a
  # Linux container would read the failure as Xcode being missing from the image.
  Scenario Outline: A kind the system does not know is refused
    Given a system configured with the sandbox kind "<kind>"
    Then the system refuses to start and names the kind it was given

    Examples:
      | kind   |
      | macosx |
      | mac    |
      | apple  |

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
