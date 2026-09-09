Feature: A transfer works when the volume is on another machine

  A volume is a directory on the machine that runs the sandboxes. The tool used to open that
  directory itself. That works while the tool and the sandboxes share one machine. Under Kubernetes
  they do not share one: the volume sits beside the control plane, and the path the system hands back
  names nothing the command can open.

  So the bytes travel through the control plane, in pieces. The command sends an address and bytes,
  and the control plane makes every decision about the filesystem. Nothing a person types changes.

  The pieces are the point. A message has a ceiling of four mebibytes, and a call that carries a
  whole file in one message only moves that ceiling somewhere else. The file below is one byte over
  it, so the runtime refuses it as a single message.

  A listing and a delete still open the directory on this machine. Both of them wait for a step of
  their own.

  Background:
    Given a running control plane

  # The same command against both, so the row that passes cannot pass because the volume was near.
  Scenario Outline: A transfer succeeds where the command cannot reach the volume, and where it can
    Given the volume is <where>
    And the system listens on an address the tool can dial
    And a workspace named "atlantic-blue"
    And a project named "vast"
    And a file of 4194305 bytes on this machine called "Explore-logs.txt"
    And that file is copied to "krewe://atlantic-blue/vast"
    When the caller copies "krewe://atlantic-blue/vast/Explore-logs.txt" out to the name "yesterday.txt"
    Then the file on this machine called "yesterday.txt" holds the bytes that were copied
    And the command succeeds

    Examples:
      | where                                                    |
      | somewhere the command cannot reach on its own filesystem |
      | on the machine the command runs on                       |

  # The refusal is the control plane's, because the control plane is the end that holds the file. It
  # still has to say what to type to mean it.
  Scenario: A name already in a volume the command cannot reach is still refused
    Given the volume is somewhere the command cannot reach on its own filesystem
    And the system listens on an address the tool can dial
    And a workspace named "atlantic-blue"
    And a project named "vast"
    And a file of 64 bytes on this machine called "explore.txt"
    And that file is copied to "krewe://atlantic-blue/vast"
    And a file of 128 bytes on this machine called "explore.txt"
    When the caller copies that file to "krewe://atlantic-blue/vast"
    Then standard error says "krewe://atlantic-blue/vast/explore.txt"
    And standard error says "--replace"
    And the command fails

  # A name that is not there reads the directory the control plane holds, so the refusal names that
  # directory rather than a path on the machine the person is typing at.
  Scenario: A file that is not in a volume the command cannot reach is a refusal that names it
    Given the volume is somewhere the command cannot reach on its own filesystem
    And the system listens on an address the tool can dial
    And a workspace named "atlantic-blue"
    And a project named "vast"
    When the caller copies "krewe://atlantic-blue/vast/nothing.txt" out to a folder on this machine
    Then standard error says "nothing.txt"
    And the command fails
