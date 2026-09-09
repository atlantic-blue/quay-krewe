Feature: A listing says what a volume holds

  A volume is the directory a session reads, and nothing said what was in one. `krewe where` names
  the directory and leaves the person to open it by hand. `krewe read` answers for a session and for
  nothing else. So a file copied into a workspace's shared folder, or into a project's folder inside
  it, could not be checked from the tool at all.

  `krewe volume list krewe://itv/vast` prints what that folder holds. The directory on the machine
  is on the first line. Under it is one line for each name, sorted by name, with a size on each file
  and a trailing slash on each folder. A folder that holds nothing says so, because an empty table
  reads as a broken command.

  The levels of the address are read against what the system holds. A segment that names no project
  and no session is the name of a file in the folder above it, so one string reaches a folder and
  reaches a file in it.

  Background:
    Given a running control plane
    And the system listens on an address the tool can dial

  Scenario: A file put in the folder by hand appears in the listing under the name it was given
    Given a workspace named "atlantic-blue"
    And a project named "vast"
    When the caller asks where "atlantic-blue/vast" is
    And a file called "explore.txt" is put in that directory by hand
    And the caller lists the volume "krewe://atlantic-blue/vast"
    Then the listing reads
      | NAME        | SIZE |
      | explore.txt | 9    |
    And the command succeeds

  # Sorted, so two reads of one directory answer the same. A size on each file, and none on a folder:
  # a folder's size on disk is the size of its own record rather than of what is in it.
  Scenario: The listing is one line for each name, sorted, with a size on each file
    Given a workspace named "atlantic-blue"
    And a project named "vast"
    When the caller asks where "atlantic-blue/vast" is
    And that directory holds
      | name      | contents |
      | beta.txt  | bbbb     |
      | logs/     |          |
      | alpha.txt | aaaaa    |
    And the caller lists the volume "krewe://atlantic-blue/vast"
    Then the listing reads
      | NAME      | SIZE |
      | alpha.txt | 5    |
      | beta.txt  | 4    |
      | logs/     |      |
    And the command succeeds

  Scenario: A folder that holds nothing says so rather than printing an empty table
    Given a workspace named "atlantic-blue"
    And a project named "vast"
    When the caller lists the volume "krewe://atlantic-blue/vast"
    Then standard output says "nothing in it"
    And the command succeeds

  # A project name and a file name are the same class of word, so the string alone cannot say which
  # this is. The system is asked, and a name that is no project is a file in the shared folder.
  Scenario: A name that is no project is read as a file in the shared folder
    Given a workspace named "atlantic-blue"
    When the caller asks where "atlantic-blue" is
    And a file called "screenshot.png" is put in that directory by hand
    And the caller lists the volume "krewe://atlantic-blue/screenshot.png"
    Then the listing reads
      | NAME           | SIZE |
      | screenshot.png | 9    |
    And the command succeeds

  # A name that is missing and a name in another folder read the same, so the refusal carries the
  # directory it read.
  Scenario: A name that is nothing at all is refused, and the refusal names the directory it read
    Given a workspace named "atlantic-blue"
    And a project named "vast"
    When the caller lists the volume "krewe://atlantic-blue/vast/nothing.txt"
    Then standard error says "nothing.txt"
    And standard error says "volume/vast"
    And the command fails
