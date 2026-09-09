Feature: A file is copied out of a volume

  A session writes its log into the volume, and until now the only way that file came back was
  `krewe read`, which holds a file to one mebibyte and answers for a session and for nothing else. A
  file bigger than that sat in a directory named in generated identifiers.

  `krewe volume cp krewe://itv/vast/Explore-logs.txt .` brings it back. It is the same verb as the
  copy in, and the scheme says which way the bytes go. That is what the scheme was added for.

  It prints one path and nothing else: the path on this machine the file landed at. A destination that
  is a directory keeps the file's own name, the way copying into a folder does everywhere else. A
  name already on this machine is refused, because a copy that writes over it silently is how the work
  in it is lost. Saying --replace means it.

  A whole directory is out of scope in this direction too. An address with no name on the end of it is
  the volume itself, and taking the first file in one would lose the rest without saying so.

  Background:
    Given a running control plane
    And the system listens on an address the tool can dial

  # The file that started this feature, at the size it is, in and then out again. It is over the one
  # mebibyte ceiling the read call holds a file to, so neither direction can be that call.
  Scenario: A file is copied in, then copied out to a new name, and the two are identical
    Given a workspace named "atlantic-blue"
    And a project named "vast"
    And a file of 1105815 bytes on this machine called "Explore-logs.txt"
    And that file is copied to "krewe://atlantic-blue/vast"
    When the caller copies "krewe://atlantic-blue/vast/Explore-logs.txt" out to the name "yesterday.txt"
    Then the file on this machine called "yesterday.txt" holds the bytes that were copied
    And standard output names the file on this machine called "yesterday.txt"
    And the command succeeds

  Scenario: A destination that is a directory keeps the file's own name
    Given a workspace named "atlantic-blue"
    And a project named "vast"
    And a file of 512 bytes on this machine called "explore.txt"
    And that file is copied to "krewe://atlantic-blue/vast"
    When the caller copies "krewe://atlantic-blue/vast/explore.txt" out to a folder on this machine
    Then the file on this machine called "explore.txt" holds the bytes that were copied
    And standard output names the file on this machine called "explore.txt"
    And the command succeeds

  # The refusal has to leave the file on this machine as it was. A refusal printed after the damage is
  # not one.
  Scenario: A name already on this machine is refused, and the file there is untouched
    Given a workspace named "atlantic-blue"
    And a project named "vast"
    And a file of 64 bytes on this machine called "explore.txt"
    And that file is copied to "krewe://atlantic-blue/vast"
    And that folder on this machine already holds a file called "explore.txt"
    When the caller copies "krewe://atlantic-blue/vast/explore.txt" out to a folder on this machine
    Then standard error says "--replace"
    And the file on this machine called "explore.txt" is the one that was already there
    And the command fails

  Scenario: Saying replace writes over the file on this machine
    Given a workspace named "atlantic-blue"
    And a project named "vast"
    And a file of 32 bytes on this machine called "explore.txt"
    And that file is copied to "krewe://atlantic-blue/vast"
    And that folder on this machine already holds a file called "explore.txt"
    When the caller copies "krewe://atlantic-blue/vast/explore.txt" out to a folder on this machine and means to replace it
    Then the file on this machine called "explore.txt" holds the bytes that were copied
    And the command succeeds

  Scenario: A name that is not in the volume is refused
    Given a workspace named "atlantic-blue"
    And a project named "vast"
    When the caller copies "krewe://atlantic-blue/vast/nothing.txt" out to a folder on this machine
    Then standard error says "nothing.txt"
    And the command fails

  # An address with no name on the end of it is the volume itself. A whole directory is out of scope.
  Scenario: The volume itself is refused rather than half copied
    Given a workspace named "atlantic-blue"
    And a project named "vast"
    When the caller copies "krewe://atlantic-blue/vast" out to a folder on this machine
    Then standard error says "one file"
    And the command fails

  # The scheme says which way the bytes go, so two of them say neither.
  Scenario: An address on both sides is refused
    Given a workspace named "atlantic-blue"
    And a project named "vast"
    When the caller copies "krewe://atlantic-blue/vast/explore.txt" to the address "krewe://atlantic-blue/vast/kept.txt"
    Then standard error says "krewe volume cp <address> <file>"
    And the command fails
