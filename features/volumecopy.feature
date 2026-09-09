Feature: A file is copied into a volume

  A person holding a file had `krewe where`, which named a directory and left them to copy into it
  by hand. There was no write verb at all. The name of that directory carries up to three generated
  identifiers, so the copy could not be typed from anything on the screen.

  `krewe volume cp ./Explore-logs.txt krewe://itv/vast` puts the file in front of every session that
  reads the address. It prints one path and nothing else: the path a session reads the file at. That
  path goes straight into what you ask the session.

  The address takes the file's own name, or it names the file itself. A name that is already there is
  refused. A copy that writes over the last one silently is how the work in it is lost. Saying
  --replace means it.

  The bytes go through one interface with one method, and the method is a stream through the control
  plane. The tool and the volume are not always on one machine, and a copy on this one reaches
  nothing where they are apart. What a person types is the same either way.

  Background:
    Given a running control plane
    And the system listens on an address the tool can dial

  # The file that started this feature, at the size it is. It is over the one mebibyte ceiling the
  # read call holds a file to. So a copy cannot be that call with a writer on the end of it.
  Scenario: A file of 1105815 bytes is copied in, and the bytes a sandbox reads match the source
    Given a workspace named "atlantic-blue"
    And a project named "vast"
    And a file of 1105815 bytes on this machine called "Explore-logs.txt"
    When the caller copies that file to "krewe://atlantic-blue/vast"
    Then a sandbox of that workspace reads that file at "/home/agent/shared/vast/Explore-logs.txt"
    And the bytes a sandbox reads at "/home/agent/shared/vast/Explore-logs.txt" are the ones that were copied
    And the command succeeds

  # The path is the whole of what this prints, because it goes into a message to a session. Anything
  # sharing the line has to be edited out by hand.
  Scenario: The copy prints the path a session reads the file at, and nothing else
    Given a workspace named "atlantic-blue"
    And a project named "vast"
    And a file of 9 bytes on this machine called "explore.txt"
    When the caller copies that file to "krewe://atlantic-blue/vast"
    Then standard output is the one path "/home/agent/shared/vast/explore.txt"
    And the command succeeds

  Scenario: A name after the folder is the name the file takes
    Given a workspace named "atlantic-blue"
    And a project named "vast"
    And a file of 32 bytes on this machine called "explore.txt"
    When the caller copies that file to "krewe://atlantic-blue/vast/yesterday.txt"
    Then standard output is the one path "/home/agent/shared/vast/yesterday.txt"
    And the bytes a sandbox reads at "/home/agent/shared/vast/yesterday.txt" are the ones that were copied
    And the command succeeds

  # The refusal has to leave the first file as it was. A refusal printed after the damage is not one.
  Scenario: A copy onto a name already there is refused, and the first file is untouched
    Given a workspace named "atlantic-blue"
    And a project named "vast"
    And a file of 64 bytes on this machine called "explore.txt"
    And that file is copied to "krewe://atlantic-blue/vast"
    And a file of 128 bytes on this machine called "explore.txt"
    When the caller copies that file to "krewe://atlantic-blue/vast"
    Then standard error says "--replace"
    And the file a sandbox reads at "/home/agent/shared/vast/explore.txt" is 64 bytes
    And the command fails

  Scenario: Saying replace writes over what is there
    Given a workspace named "atlantic-blue"
    And a project named "vast"
    And a file of 4096 bytes on this machine called "explore.txt"
    And that file is copied to "krewe://atlantic-blue/vast"
    And a file of 32 bytes on this machine called "explore.txt"
    When the caller copies that file to "krewe://atlantic-blue/vast" and means to replace it
    Then the bytes a sandbox reads at "/home/agent/shared/vast/explore.txt" are the ones that were copied
    And the command succeeds

  # A whole directory is out of scope. Taking the first file in one would be worse than refusing.
  # Nobody would learn the other files never went.
  Scenario: A whole directory is refused rather than half copied
    Given a workspace named "atlantic-blue"
    And a project named "vast"
    And a file of 8 bytes on this machine called "explore.txt"
    When the caller copies the folder that file is in to "krewe://atlantic-blue/vast"
    Then standard error says "directory"
    And the command fails
