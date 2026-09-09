Feature: A file is deleted from a volume

  A volume only ever grew. Every other verb puts something in one or reads what is in it, so a file
  copied in by mistake, or one that is finished with, stayed where every session of the workspace
  reads it. The only way to take it back was to open the directory by hand, and the name of that
  directory carries up to three generated identifiers.

  `krewe volume delete krewe://itv/vast/Explore-logs.txt` removes it. One named file goes and
  nothing beside it does. It prints the path the file was at, because the levels of an address are
  read against what the system holds, and `krewe://itv/notes` is a project or a file in the shared
  folder.

  A folder is refused rather than emptied. A recursive delete is out of scope, and there is no way
  back from one: an address with the file name left off names the volume itself, which is the shape
  somebody types by accident.

  A name that is not there is refused, and the refusal says what the directory does hold. A delete is
  typed from memory, so the file is usually there under another name.

  Background:
    Given a running control plane
    And the system listens on an address the tool can dial

  # A delete is typed from memory, so the names beside the missing one are the answer. Without them
  # the person has to list the directory themselves to learn what they meant to type.
  Scenario: Deleting a name that is not there says what the directory holds, and fails
    Given a workspace named "atlantic-blue"
    And a project named "vast"
    When the caller asks where "atlantic-blue/vast" is
    And that directory holds
      | name        | contents |
      | explore.txt | eeee     |
      | logs/       |          |
    And the caller deletes "krewe://atlantic-blue/vast/Explore-logs.txt"
    Then standard error says "Explore-logs.txt"
    And standard error says "explore.txt"
    And standard error says "logs/"
    And the command fails

  Scenario: One named file goes and nothing beside it does
    Given a workspace named "atlantic-blue"
    And a project named "vast"
    When the caller asks where "atlantic-blue/vast" is
    And that directory holds
      | name        | contents |
      | explore.txt | eeee     |
      | kept.txt    | kkk      |
    And the caller deletes "krewe://atlantic-blue/vast/explore.txt"
    Then a sandbox of that workspace reads no file at "/home/agent/shared/vast/explore.txt"
    And standard output is the one path "/home/agent/shared/vast/explore.txt"
    And the command succeeds
    When the caller lists the volume "krewe://atlantic-blue/vast"
    Then the listing reads
      | NAME     | SIZE |
      | kept.txt | 3    |
    And the command succeeds

  # Emptying a folder takes every file under it, and nothing here brings any of them back.
  Scenario: A folder in the volume is refused rather than emptied
    Given a workspace named "atlantic-blue"
    And a project named "vast"
    When the caller asks where "atlantic-blue/vast" is
    And that directory holds
      | name  | contents |
      | logs/ |          |
    And the caller deletes "krewe://atlantic-blue/vast/logs"
    Then standard error says "is a folder, and this deletes one file"
    And the command fails
    When the caller lists the volume "krewe://atlantic-blue/vast"
    Then the listing reads
      | NAME  | SIZE |
      | logs/ |      |

  # An address with the file name left off is the volume itself, which is the shape somebody types by
  # accident.
  Scenario: The volume itself is refused rather than emptied
    Given a workspace named "atlantic-blue"
    And a project named "vast"
    When the caller asks where "atlantic-blue/vast" is
    And that directory holds
      | name        | contents |
      | explore.txt | eeee     |
    And the caller deletes "krewe://atlantic-blue/vast"
    Then standard error says "is a volume, and this deletes one file"
    And the command fails
    When the caller lists the volume "krewe://atlantic-blue/vast"
    Then the listing reads
      | NAME        | SIZE |
      | explore.txt | 4    |
