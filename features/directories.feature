Feature: A name becomes a directory, so a person can put a file in front of a session

  Every level the system keeps on disk is a generated identifier. The names are in the control plane
  and none of them is on the filesystem, so somebody who knows they work in `atlantic-blue` has
  nothing to type. Putting a screenshot in front of a running session meant reading three directories
  named in hex, then inspecting a container that happened to be up to learn that a workspace's volume
  is bound at `/home/agent/shared`.

  That last step is the one with no answer at all. A bind mount can be read off a live container, and
  the question is usually asked once every container is down.

  So an address answers with a directory. A workspace address answers with its shared folder, which
  every session in it reads. A project address answers with a folder inside that one, named after the
  project. A session address answers with where that session's work is, which is the working tree it
  took where it took one and its own working directory where it did not. The path is on the first line
  with nothing beside it, so it can be typed into a shell, and under it is where a session sees the
  same directory, which is what to call the file once it is in there.

  The answer names which of the two roots it read, because they are two directories and only the
  sentence tells them apart.

  The project folder is where sessions were already putting a project's files, by hand. A workspace
  address and a project address used to answer with one directory between them, so the two addresses
  said different things and reached the same place.

  The directory is made if it is not there. A workspace nobody has worked in has no folder yet,
  because the folder is made when a sandbox starts, and a path that cannot be copied into is not an
  answer to somebody holding a file.

  One directory is never named. The top of the data directory holds the system's token, the driver's
  token and the key that unseals every secret, so the word that would name it is refused and says why.

  Background:
    Given a running control plane
    And the system listens on an address the tool can dial

  Scenario: A workspace nobody has worked in yet still has a folder to put a file in
    Given a workspace named "atlantic-blue"
    When the caller asks where "atlantic-blue" is
    Then the directory it names exists on the machine
    And it says a session reads that directory at "/home/agent/shared"

  Scenario: The path is alone on the first line, so it can be typed into a shell
    Given a workspace named "atlantic-blue"
    When the caller asks where "atlantic-blue" is
    Then the first line is a path and nothing else

  Scenario: A file put in that directory by hand is where the session will look for it
    Given a workspace named "atlantic-blue"
    When the caller asks where "atlantic-blue" is
    And a file called "screenshot.png" is put in that directory by hand
    Then a sandbox of that workspace binds that directory at "/home/agent/shared"
    And the file is inside the directory that sandbox binds

  Scenario: A file put in the project folder is read by a sandbox of that workspace
    Given a workspace named "atlantic-blue"
    And a project named "vast"
    When the caller asks where "atlantic-blue/vast" is
    Then the directory it names exists on the machine
    And it says a session reads that directory at "/home/agent/shared/vast"
    And a file called "explore.txt" is put in that directory by hand
    Then a sandbox of that workspace reads that file at "/home/agent/shared/vast/explore.txt"

  # The old answer, kept beside the new one so the change is visible rather than silent.
  Scenario: A workspace address still answers with the shared folder itself
    Given a workspace named "atlantic-blue"
    And a project named "vast"
    When the caller asks where "atlantic-blue/vast" is
    And the caller asks where "atlantic-blue" is
    Then it says a session reads that directory at "/home/agent/shared"
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
    When the caller asks where "nowhere" is
    Then standard error says "atlantic-blue"
    And the command fails

  # The tokens and the sealing key are at the top of the data directory. A command that answers "where
  # do I put a file" must not offer a road to them.
  Scenario: The system's own directory is refused, and says what is in it
    Given a workspace named "atlantic-blue"
    When the caller asks where "system" is
    Then standard error says "credentials"
    And the command fails

  # The git skill teaches a session to take a working tree in the workspace's volume, under its own
  # identifier, so the session's own directory stays empty. A session address that named the empty one
  # sent a person to copy a file into a directory nothing was working in, and reading it back said the
  # session had made nothing.
  Scenario: A session takes a working tree, and its address lists the checkout rather than an empty directory
    Given a workspace named "atlantic-blue"
    And a project named "vast"
    And a session started by dispatching "clone the repository"
    And that session takes a working tree holding a checkout called "quay-krewe"
    When the caller asks where that session is
    Then the directory it names holds the folder "quay-krewe"
    And it says the directory is that session's working tree
    And that session reads the directory it names at the mount the answer promised
    And that session's own directory holds nothing
