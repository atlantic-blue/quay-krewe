Feature: A listing works when the volume is on another machine

  A volume is a directory on the machine that runs the sandboxes. The listing asked the system where
  that directory was and then opened the path itself. That works while the tool and the sandboxes
  share one machine. Under Kubernetes they do not: the volume sits beside the control plane, and the
  path the system hands back names nothing the command can open.

  So the listing came back saying the directory held nothing, which reads exactly like a volume
  nobody has copied into yet. The names come from the control plane now, the way the bytes of a copy
  already do. Nothing a person types changes.

  The key travels with the request and is held inside the volume at the end that reads it. The
  command cannot be the end that holds it any more, because the command never sees the directory.

  A delete still opens the directory on this machine. It waits for a step of its own.

  Background:
    Given a running control plane

  # The same command against both, so the row that passes cannot pass because the volume was near.
  Scenario Outline: A listing names every file in the volume, where the command cannot reach it and where it can
    Given the volume is <where>
    And the system listens on an address the tool can dial
    And a workspace named "atlantic-blue"
    And a project named "vast"
    And a file of 4 bytes on this machine called "beta.txt"
    And that file is copied to "krewe://atlantic-blue/vast"
    And a file of 5 bytes on this machine called "alpha.txt"
    And that file is copied to "krewe://atlantic-blue/vast"
    When the caller lists the volume "krewe://atlantic-blue/vast"
    Then the listing reads
      | NAME      | SIZE |
      | alpha.txt | 5    |
      | beta.txt  | 4    |
    And the command succeeds

    Examples:
      | where                                                    |
      | somewhere the command cannot reach on its own filesystem |
      | on the machine the command runs on                       |

  # A volume nobody has copied into and a volume nobody can reach used to read the same. This is the
  # first of the two, and it is the one that has to keep reading that way.
  Scenario: An empty volume the command cannot reach says it holds nothing
    Given the volume is somewhere the command cannot reach on its own filesystem
    And the system listens on an address the tool can dial
    And a workspace named "atlantic-blue"
    And a project named "vast"
    When the caller lists the volume "krewe://atlantic-blue/vast"
    Then standard output says "nothing in it"
    And the command succeeds

  # A key naming one file lists that file, which is the command somebody types to check a copy
  # arrived under the name they gave it.
  Scenario: A key that names one file in a volume the command cannot reach lists that file
    Given the volume is somewhere the command cannot reach on its own filesystem
    And the system listens on an address the tool can dial
    And a workspace named "atlantic-blue"
    And a project named "vast"
    And a file of 9 bytes on this machine called "explore.txt"
    And that file is copied to "krewe://atlantic-blue/vast"
    When the caller lists the volume "krewe://atlantic-blue/vast/explore.txt"
    Then the listing reads
      | NAME        | SIZE |
      | explore.txt | 9    |
    And the command succeeds

  # The oldest way into a directory that is not yours. What this proves is the refusal a person gets:
  # it comes from the control plane now, and it names the volume rather than anything above it.
  #
  # It does not prove the guard. The tool cleans a key while it reads the address, so a key that
  # climbs cannot reach the control plane through the tool at all, and this scenario passes against a
  # control plane with no guard in it. The guard is proved where it lives, on a request built the way
  # anything else on the wire builds one: TestListVolumeHoldsAKeyThatClimbsInsideTheVolume.
  Scenario: A key that climbs out of a volume the command cannot reach is refused
    Given the volume is somewhere the command cannot reach on its own filesystem
    And the system listens on an address the tool can dial
    And a workspace named "atlantic-blue"
    And a project named "vast"
    When the caller lists the volume "krewe://atlantic-blue/vast/../../etc/passwd"
    Then standard error says "etc/passwd"
    And standard error says "volume/vast"
    And the command fails
