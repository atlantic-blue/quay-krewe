Feature: Discovery writes down what a repository already holds

  A project with a repository does not start from nothing. It starts from routes somebody already
  built, components somebody already named, and colours already written down in a file. The six
  design stages had no way in for any of that, so the first stage of an existing project was a blank
  page, and the stage after it designed a screen the repository already draws.

  Discovery is that first stage, and `/krewe:discover` is how the operator asks for it. The operator
  types one word. A session in a sandbox reads the repository, writes down six lists, and gives each
  item the file it came from. Beside the lists it writes a draft `flows.json` of the screens that
  exist today, each one marked built, each one naming its surface and the file it was read from.

  The operator reads both documents and approves the stage. Approving stays the operator's word: a
  session that could agree with its own reading of a repository would hand the next five stages a
  list nobody checked.

  Background:
    Given a running control plane
    And the system listens on an address the tool can dial
    And a workspace named "acme"
    And a project named "house-bills"

  # The one this slice exists for. Three routes in the repository, three screens in the artifact, and
  # every one of them marked as something that exists rather than something somebody wants.
  Scenario: Discovery lists the screens of an existing repository
    Given a repository holding these files:
      """
      app/routes/bills.tsx
      app/routes/bill.tsx
      app/routes/settings.tsx
      """
    When a session writes the discovery that the discover skill shows
    Then the discovery stage names every file of that repository
    And the discovery artifact holds 3 screens
    And every screen in the discovery artifact was built already
    And every screen in the discovery artifact names its surface
    And every screen in the discovery artifact names a file that repository holds

  # Discovery is the first of the six, so nothing else can be written until it carries the word. A
  # discovery that the operator approved is what opens the stage after it.
  Scenario: The stage after discovery opens once the operator approves it
    Given a repository holding these files:
      """
      app/routes/bills.tsx
      app/routes/bill.tsx
      app/routes/settings.tsx
      """
    And a session writes the discovery that the discover skill shows
    When the operator approves the "discovery" design stage
    And the operator writes the "stories" design stage as "one story for each screen that exists"
    Then the operator reads the project's design stages
    And the "stories" design stage reads "one story for each screen that exists"

  # What the session reads before it reads a repository. The brief is prose and nothing else: no gate
  # reads it, so what it fails to say is a thing the discovery will not carry.
  Scenario Outline: The discover skill says what a discovery must hold
    When the operator reads the discover skill
    Then the discover skill says "<said>"

    Examples:
      | said                                             |
      | the file each thing came from                    |
      | Routes and screens                               |
      | Components                                       |
      | Tokens                                           |
      | Data entities                                    |
      | Patterns                                         |
      | Test tiers                                       |
      | The status reads `built`                         |
      | The surface reads `web` or `mobile`              |
      | The source names the file you read the screen from |
      | Only the operator approves                       |

  # The example is the part a session copies, so a broken one teaches every session the wrong shape.
  Scenario: The example beside the brief is a whole discovery of three screens
    When the operator reads the discover skill
    Then the discover skill ships the example it names
    And the example artifact holds 3 screens
    And every screen in the example was built already

  # The operator's own terminal. It asks, it dispatches, it reads back, and the session in the
  # sandbox is what writes: a discovery written in the operator's terminal has no sandbox, no record
  # of what was read, and no session to answer for it afterwards.
  Scenario: The discover command ships with the set, and carries the marker and one description
    Given the operator's agent reads its commands from a directory of its own
    When the operator installs the slash commands
    Then the command succeeds
    And the installed command "discover" carries the marker of this build
    And the installed command "discover" describes itself in one line

  Scenario: The discover command dispatches a session that holds the discover skill
    Given the operator's agent reads its commands from a directory of its own
    When the operator installs the slash commands
    Then the command succeeds
    And the installed command "discover" names "krewe skill list"
    And the installed command "discover" names "krewe skill attach <workspace> discover"
    And the installed command "discover" names "krewe exec --dispatch"
    And the installed command "discover" names "krewe sessions"
    And the installed command "discover" names "krewe stage show <workspace>/<project>"

  # Reading it back is the whole of the operator's side. The stage listing says written and says how
  # long the document is; it never prints the document, so the command brings both files out of the
  # project's volume and prints the discovery whole.
  Scenario: The discover command brings the two documents back and prints the discovery whole
    Given the operator's agent reads its commands from a directory of its own
    When the operator installs the slash commands
    Then the command succeeds
    And the installed command "discover" names "krewe answer"
    And the installed command "discover" names "krewe volume cp krewe://<workspace>/<project>/discovery.md ."
    And the installed command "discover" names "krewe volume cp krewe://<workspace>/<project>/flows.json ."
    And the installed command "discover" says it prints the discovery whole

  Scenario: The discover command asks for a yes before it approves
    Given the operator's agent reads its commands from a directory of its own
    When the operator installs the slash commands
    Then the command succeeds
    And the installed command "discover" names "krewe stage approve"
    And the installed command "discover" asks for a yes where it runs "krewe stage approve"
    And the installed command "discover" says a no leaves the discovery unapproved

  Scenario: The discover command writes no stage and no design of its own
    Given the operator's agent reads its commands from a directory of its own
    When the operator installs the slash commands
    Then the command succeeds
    And the installed command "discover" runs no command that writes a design or a path
    And the installed command "discover" carries no design document of its own

  # A repository is read before anybody designs anything, so the word sits between starting the
  # project and designing it.
  Scenario: The listing names discover between init and design
    Given the operator's agent reads its commands from a directory of its own
    When the operator asks which slash commands this build carries
    Then the command succeeds
    And standard output names "/krewe:init" before "/krewe:discover"
    And standard output names "/krewe:discover" before "/krewe:design"
