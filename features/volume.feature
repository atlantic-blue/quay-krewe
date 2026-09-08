Feature: A volume is addressed by one string

  A file reaches a session by going into a directory that session reads. Every level of that
  directory on disk is a generated identifier, so an address names it instead. `krewe://itv` is the
  shared folder of the itv workspace. `krewe://itv/vast` is the vast folder inside it.
  `krewe://itv/vast/9e8153f6/a.txt` is one file in one session's own directory.

  The scheme tells an address from a path on the machine, because a copy takes one of each. The
  levels come first and the name of the file is the rest. One thing in the string is structural: a
  session identifier is hexadecimal, and 8 to 24 characters. Every other segment could be a name or
  the name of a file, so the address is read as deep as it goes and the guess is marked. The
  resolver settles a marked reading against what the system holds. It takes the longest prefix of
  the levels that is really there, and the rest is the name of the file.

  Scenario: An address and a file name are read out of one string
    When the operator reads the volume address "krewe://itv/vast/logs/explore.txt"
    Then the address names the workspace "itv"
    And the address names the project "vast"
    And the address names the session "logs"
    And the address names the file "explore.txt"

  Scenario: A session identifier settles the reading, and nothing is looked up
    When the operator reads the volume address "krewe://itv/vast/9e8153f6/explore.txt"
    Then the address names the session "9e8153f6"
    And the address names the file "explore.txt"
    And the reading is settled

  # A project name and a file name are the same class of word, so the string cannot say which this is.
  Scenario: A plain word is filled as a session and marked for the resolver
    When the operator reads the volume address "krewe://itv/vast/notes"
    Then the address names the session "notes"
    And the reading is a guess from the project down

  # A channel names a conversation whatever way it likes, so a handle is not shaped like anything
  # this system makes.
  Scenario: A handle a channel chose is filled as a session and marked for the resolver
    When the operator reads the volume address "krewe://itv/vast/C0FFEE.99"
    Then the address names the session "C0FFEE.99"
    And the reading is a guess from the project down

  Scenario: An address stops at a folder and names no file
    When the operator reads the volume address "krewe://itv/vast"
    Then the address names the project "vast"
    And the address names no file

  # The oldest way into a directory that is not yours. The key is cleaned onto the root of nothing,
  # so it lands inside the volume or it lands nowhere. The two path elements never fill a level
  # either, which is what keeps them under the cleaning.
  Scenario: A key of `../../etc/passwd` resolves to a name inside the directory and never outside it
    When the operator reads the volume address "krewe://itv/vast/../../etc/passwd"
    Then the address names the project "vast"
    And the address names the file "etc/passwd"

  # The directory a session's working tree goes in sits beside the project folders, so a project of
  # that name would be one path naming two directories.
  Scenario: A project called worktrees is refused, and the refusal says how to reach one
    When the operator reads the volume address "krewe://itv/worktrees"
    Then the volume address is refused
    And the refusal names a session address

  Scenario: The system's own directory is not a volume
    When the operator reads the volume address "krewe://system"
    Then the volume address is refused
    And the refusal says the directory holds the tokens
