Feature: Krewe puts its own slash commands in the operator's terminal

  The operator drives this system from a terminal, and the first thing anybody does in a new project
  is the same few commands in the same order. Nothing shipped them. Every operator typed the order
  out again from the manual, or got it wrong.

  So krewe carries the commands its own agent reads, as markdown files in the binary, and writes them
  where the agent looks for them. `krewe commands install` is the whole of it. The directory name is
  the namespace, so a file called init.md becomes /krewe:init.

  The install is the part worth being careful about, because it writes into a directory that is the
  operator's and not krewe's. Line one of every file krewe writes is a marker naming the build that
  wrote it. A file carrying the marker is replaced. A file carrying no marker is somebody else's
  work, and one of those refuses the whole install, before the first write, and names the file. There
  is no flag that goes over the refusal: a file krewe did not write stays until somebody removes it
  themselves.

  Nothing here touches the store. There is no table, no migration and no call, and none of these
  words takes an address: the files belong to the machine rather than to a project.

  These scenarios run the real tool in its own process, against a command directory of their own, so
  what is proved is what an operator gets. The rules that hold over every file, and that every verb a
  file names is a verb the tool has, are read over the whole embedded set in internal/commands. What
  one command says for itself is read here, out of the file the install put on the machine.

  Background:
    Given a running control plane
    And the system listens on an address the tool can dial
    And the operator's agent reads its commands from a directory of its own

  Scenario: An install writes every command this build carries, and says where each one went
    When the operator installs the slash commands
    Then the command succeeds
    And the directory holds every command this build carries
    And it names where every command went
    And it says how many it wrote

  # An upgrade moves the binary and leaves the files where they were, so the ordinary second install
  # is the one that catches up. It replaces what krewe wrote, whatever build the marker names.
  Scenario: A second install replaces the files krewe wrote and refuses nothing
    Given the slash commands are installed
    When the operator installs the slash commands
    Then the command succeeds
    And the directory holds every command this build carries
    And every command in the directory names this build

  # The scenario this whole slice exists for. A refusal that had already written half the set is not
  # a refusal, so the check reads every file it would write before it writes the first one.
  Scenario: An install over a file the operator wrote refuses, names it, and writes nothing at all
    Given the operator wrote "init.md" in that directory by hand
    When the operator installs the slash commands
    Then the command fails
    And it names the file the operator wrote
    And standard error says "krewe commands install"
    And the file the operator wrote says what it said before
    And nothing else was written into that directory

  Scenario: A file krewe never named survives an install
    Given the operator wrote "mine.md" in that directory by hand
    When the operator installs the slash commands
    Then the command succeeds
    And the file the operator wrote says what it said before

  Scenario: Nothing installed names the directory and says how to install
    When the operator asks where the slash commands go
    Then the command fails
    And it names the command directory
    And standard error says "krewe commands install"

  Scenario: After an install the files and the tool are the same build
    Given the slash commands are installed
    When the operator asks where the slash commands go
    Then the command succeeds
    And it prints this build twice
    And it never says to install again

  Scenario: Files from an older build say to install again
    Given the directory holds "init.md" written by build "older11"
    When the operator asks where the slash commands go
    Then the command succeeds
    And standard output says "older11"
    And standard output says "Run krewe commands install"

  Scenario: A file with no marker is listed with the word unknown where the build goes
    Given the operator wrote "init.md" in that directory by hand
    When the operator asks where the slash commands go
    Then the command succeeds
    And standard output says "unknown"

  # The listing reads the files in the binary, so it says what this build would write rather than
  # what is on the machine. That is why it answers before anything is installed.
  Scenario: The listing answers on a machine where nothing was installed
    When the operator asks which slash commands this build carries
    Then the command succeeds
    And it names every command this build carries, with what each one does
    And nothing at all was written into that directory

  # An absence proved by not writing the code is one that the next person to think a force flag would
  # be helpful can undo with nothing going red.
  Scenario Outline: No flag writes over a file krewe did not write
    Given the operator wrote "init.md" in that directory by hand
    When the operator installs the slash commands with "<flag>"
    Then the command fails
    And the file the operator wrote says what it said before

    Examples:
      | flag        |
      | --force     |
      | --replace   |
      | --overwrite |
      | --anyway    |
      | --yes       |

  # A slash command is a file the operator's own terminal runs, on their word, outside every sandbox.
  # So a session that could write one could hand them a command that does anything at all. The
  # refusal is written in the same list every other refusal a session meets is written in.
  Scenario: A driver session that tries to install is refused
    Given the tool is running inside a session's sandbox
    When the operator installs the slash commands
    Then the command fails
    And standard error says "the operator's to make"
    And nothing at all was written into that directory

  # /krewe:design is the command that carries the design conversation, and it is the one where
  # writing the design in the operator's own terminal would look like helpfulness. It is the wrong
  # place: a design written there has no sandbox, no record of what was read, and no session to
  # answer for it afterwards. So the file is read for the two shapes that would break the rule.
  Scenario: The design command writes no design body and no path
    When the operator installs the slash commands
    Then the command succeeds
    And the installed command "design" runs no command that writes a design or a path
    And the installed command "design" carries no design document of its own

  Scenario: The design command ships with the set, and carries the marker and one description
    When the operator installs the slash commands
    Then the command succeeds
    And the installed command "design" carries the marker of this build
    And the installed command "design" describes itself in one line

  # The order the commands are met in, which is not the order a directory read gives back.
  Scenario: The listing names init first and design second
    When the operator asks which slash commands this build carries
    Then the command succeeds
    And standard output names "/krewe:init" before "/krewe:design"

  # The design work belongs to a session in a sandbox. The command makes sure that session holds the
  # design skill, dispatches it, and then reads back what it wrote.
  Scenario: The design command dispatches a session that holds the design skill
    When the operator installs the slash commands
    Then the command succeeds
    And the installed command "design" names "krewe skill list"
    And the installed command "design" names "krewe skill attach"
    And the installed command "design" names "krewe exec --dispatch"
    And the installed command "design" names "krewe sessions"
    And the installed command "design" names "krewe design <workspace>/<project>"

  # Approval is the operator's word. The yes sits beside the command that gives it, so taking the
  # question out of that step is what this scenario reads.
  Scenario: The design command asks for a yes before it approves
    When the operator installs the slash commands
    Then the command succeeds
    And the installed command "design" names "krewe design approve"
    And the installed command "design" asks for a yes where it runs "krewe design approve"
    And the installed command "design" says a no leaves the design unapproved
