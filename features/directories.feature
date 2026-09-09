Feature: A name becomes a directory, so a person can put a file in front of a session

  Every level the system keeps on disk is a generated identifier. The names are in the control plane
  and none of them is on the filesystem, so somebody who knows they work in `atlantic-blue` has
  nothing to type. Putting a screenshot in front of a running session meant reading three directories
  named in hex, then inspecting a container that happened to be up to learn that a workspace volume
  is bound at `/home/agent/shared`.

  That last step is the one with no answer at all. A bind mount can be read off a live container, and
  the question is usually asked once every container is down.

  So an address is a volume, and the volume verb prints the directory it is kept in on this machine.
  A workspace address answers with its shared folder, which every session in it reads. A project
  address answers with a folder inside that one, named after the project. A session address answers
  with what that session works in, which is the working tree it took where it took one and its own
  directory where it did not. The path is on the first line with nothing beside it, so it can be
  typed into a shell.

  The project folder is where sessions were already putting a project files, by hand. A workspace
  address and a project address used to answer with one directory between them, so the two addresses
  said different things and reached the same place.

  The directory is made if it is not there. A workspace nobody has worked in has no folder yet,
  because the folder is made when a sandbox starts, and a path that cannot be copied into is not an
  answer to somebody holding a file.

  One directory is never named. The top of the data directory holds the system token, the driver
  token and the key that unseals every secret, so the word that would name it is refused and says why.

  Two words did this before, and both are gone. `krewe where` named a directory and stopped there.
  `krewe read` answered for a session and for nothing else, and it held a file to one mebibyte. The
  volume verb does all of it, and the tree under `~/.krewe/at` puts the same directories under their
  names for a file manager. Each word refuses by name and says what to type, because both are in
  fingers, in scripts and in notes.

  One thing is removed rather than moved: `krewe where` with no address named the directory of
  wherever you were standing, and no command answers that now. The volume verb takes an address.

  Background:
    Given a running control plane
    And the system listens on an address the tool can dial

  Scenario: A workspace nobody has worked in yet still has a folder to put a file in
    Given a workspace named "atlantic-blue"
    When the caller lists the volume "krewe://atlantic-blue"
    Then the directory it names exists on the machine
    And a sandbox of that workspace binds that directory at "/home/agent/shared"

  Scenario: The path is alone on the first line, so it can be typed into a shell
    Given a workspace named "atlantic-blue"
    When the caller lists the volume "krewe://atlantic-blue"
    Then the first line is a path and nothing else

  Scenario: A file put in that directory by hand is where the session will look for it
    Given a workspace named "atlantic-blue"
    When the caller lists the volume "krewe://atlantic-blue"
    And a file called "screenshot.png" is put in that directory by hand
    Then a sandbox of that workspace binds that directory at "/home/agent/shared"
    And the file is inside the directory that sandbox binds

  Scenario: A file put in the project folder is read by a sandbox of that workspace
    Given a workspace named "atlantic-blue"
    And a project named "vast"
    When the caller lists the volume "krewe://atlantic-blue/vast"
    Then the directory it names exists on the machine
    And a file called "explore.txt" is put in that directory by hand
    Then a sandbox of that workspace reads that file at "/home/agent/shared/vast/explore.txt"

  # The old answer, kept beside the new one so the change is visible rather than silent.
  Scenario: A workspace address still answers with the shared folder itself
    Given a workspace named "atlantic-blue"
    And a project named "vast"
    When the caller lists the volume "krewe://atlantic-blue/vast"
    And the caller lists the volume "krewe://atlantic-blue"
    Then a sandbox of that workspace binds that directory at "/home/agent/shared"
    And the directory it names holds the folder "vast"

  # A project folder sits beside the folders the system writes in the shared volume itself, so a
  # project of one of those names would be one path naming two directories.
  Scenario: A project cannot be called a folder the system already writes
    Given a workspace named "atlantic-blue"
    When the caller makes a project called "worktrees"
    Then standard error says "shared folder"
    And the command fails

  Scenario: An address that does not exist says what there is
    Given a workspace named "atlantic-blue"
    When the caller lists the volume "krewe://nowhere"
    Then standard error says "atlantic-blue"
    And the command fails

  # The tokens and the sealing key are at the top of the data directory. A command that answers "what
  # is in this directory" must not offer a road to them.
  Scenario: The system's own directory is refused, and says what is in it
    Given a workspace named "atlantic-blue"
    When the caller lists the volume "krewe://system"
    Then standard error says "the sealing key"
    And the command fails

  # The git skill teaches a session to take a working tree in the workspace volume, under its own
  # identifier, so the session own directory stays empty. A session address that named the empty one
  # sent a person to copy a file into a directory nothing was working in, and reading it back said the
  # session had made nothing.
  Scenario: A session takes a working tree, and its address lists the checkout rather than an empty directory
    Given a workspace named "atlantic-blue"
    And a project named "vast"
    And a session started by dispatching "clone the repository"
    And that session takes a working tree holding a checkout called "quay-krewe"
    When the caller lists the volume of that session
    Then the directory it names holds the folder "quay-krewe"
    And that session reads the directory it names at the mount a sandbox is given
    And that session's own directory holds nothing

  # The way off the two words that did this before. Both are in fingers and in notes, so each one
  # refuses by name, exits non zero, and says what to type. A word that becomes an unknown command
  # reads as the tool being broken, and a word that is quietly accepted is worse than both.
  Scenario Outline: A word that went refuses, names the volume verb, and fails
    Given a workspace named "atlantic-blue"
    When the caller types "<gone>" through the tool
    Then standard error says "there is no <word> command"
    And standard error says "krewe volume"
    And standard output is empty
    And the command fails

    Examples:
      | gone                        | word  |
      | where                       | where |
      | where atlantic-blue         | where |
      | read atlantic-blue          | read  |
