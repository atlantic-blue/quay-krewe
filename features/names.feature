Feature: Every address has a name on the filesystem

  Every level this system keeps on disk is a generated identifier. A workspace's shared folder is
  workspaces/<24 hexadecimal characters>/volume, and a session's own directory is three of those
  deep. The names are in the store and nowhere on the machine, so the finder showed identifiers and
  the only way to open the right folder was to ask the tool for a path and paste it back.

  So the system writes a tree of names beside its data directory, at ~/.krewe/at. A workspace's name
  is a link to its shared folder, so ~/.krewe/at/itv is what every session of itv reads, and
  ~/.krewe/at/itv/vast is the vast project's folder inside it. One link carries the workspace and
  every project in it, because a project's folder is already named after the project.

  A session's own directory is not inside that shared folder, so its name sits beside the workspace's
  rather than under it: a link written inside the folder points at a path on the host, and every
  container that reads the folder sees a name pointing at nothing.

  The tree is a view. It is built from what the store holds, nothing reads a name back out of it, and
  deleting the whole tree costs the names and no work. The identifiers stay canonical underneath,
  because a name changes and an identifier does not.

  Background:
    Given a running control plane

  Scenario: A drop into the name of a project lands in the folder a sandbox of that workspace binds
    Given a workspace named "itv"
    And a project named "vast"
    When a file called "explore.txt" is dropped into the name "itv/vast"
    Then a sandbox of that workspace reads that file at "/home/agent/shared/vast/explore.txt"

  Scenario: A workspace's name is its shared folder, and every project is a folder inside it
    Given a workspace named "itv"
    And a project named "vast"
    When a file called "notes.md" is dropped into the name "itv"
    Then a sandbox of that workspace reads that file at "/home/agent/shared/notes.md"
    And the name "itv" holds the folder "vast"

  # The session level is the one the tree has to carry itself, because a session's directory is not
  # in the volume any of these other names reach.
  Scenario: A session's name reaches the session's own directory
    Given a workspace named "itv"
    And a project named "vast"
    When a session is dispatched with the title "the login that times out"
    And a file called "screenshot.png" is dropped into the name "itv.sessions/vast/the-login-that-times-out"
    Then a sandbox of that session reads that file at "/home/agent/workspace/screenshot.png"

  # A session nobody has called anything has only its identifier left, and the tree holds names.
  Scenario: A session with no name of its own is left out rather than filed under its identifier
    Given a workspace named "itv"
    And a project named "vast"
    When a session started by dispatching "look at the login"
    Then the tree of names holds no identifier
