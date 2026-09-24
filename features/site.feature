Feature: A project's stages are read over http

  The six design stages sit in the database. The only way to read one was krewe stage show, in a
  terminal, one stage at a time. So the operator approved a design by reading prose in a scrollback.

  This gives the stages an address. The control plane opens a second listener, plain http on
  127.0.0.1 port 50052. It answers two documents: the stages of a project, and the artifact one stage
  carries. Both are json, and a page reads them.

  The listener is read only. It carries no token, because it writes nothing and it approves nothing.
  It binds loopback, because the address is the whole system and a port is not a boundary. The
  compose stack publishes it on 127.0.0.1 alone.

  An address that names nothing says which part of it named nothing: the workspace, the project or
  the stage. A method that writes is refused the same way, because the surface has no write on it.

  The address also serves a page. It lists Design and then the six stages, and each entry carries one
  word: approved, changed since approval, written, not written or skipped. A stage the operator
  agreed to reads differently from one written again since then, so the word on the screen is the
  word that still stands.

  Background:
    Given a running control plane
    And a workspace named "acme"
    And a project named "house-bills"

  # The one this step exists for. Two stages are written and agreed, and the operator reads them back
  # from an address. Then an address for a project nobody made says so.
  Scenario: The site answers the stages a project has written
    Given the operator sets the project's brief to "four bills, and two of them move"
    And the "discovery" design stage is written and approved
    And the "stories" design stage is written and approved
    And the site is served on a local address
    When the operator reads the site at "/p/acme/house-bills/stages.json"
    Then the site answers 200
    And the site answers json
    And the answer carries the brief "four bills, and two of them move"
    And the answer carries the "discovery" stage, written and approved at the same version
    And the answer carries the "stories" stage, written and approved at the same version
    And the answer says the "stories" stage reads "the stories body"
    When the operator reads the site at "/p/acme/no-such-project/stages.json"
    Then the site answers 404
    And the answer says "no-such-project"

  # A stage written again. The version moves, so the word no longer stands, and the version it was
  # given to stays on the row. Both numbers travel, so the page can say which of the three a stage is:
  # agreed, written and never agreed, or changed after the word was given.
  Scenario: A stage written again comes back without the word it had
    Given the "discovery" design stage is written and approved
    And the operator writes the "discovery" design stage as "four bills, and three of them move"
    And the site is served on a local address
    When the operator reads the site at "/p/acme/house-bills/stages.json"
    Then the site answers 200
    And the answer carries the "discovery" stage at version 2, approved at version 1
    And the answer says the "discovery" stage reads "four bills, and three of them move"

  # The workspace, the project and the stage each name themselves when they are the part that is
  # missing, because an operator told only "not found" has three places to go and look.
  Scenario Outline: An address that names nothing says which part named nothing
    Given the "discovery" design stage is written and approved
    And the site is served on a local address
    When the operator reads the site at "<address>"
    Then the site answers 404
    And the answer says "<said>"

    Examples:
      | address                                      | said            |
      | /p/no-such-workspace/house-bills/stages.json | no-such-workspace |
      | /p/acme/no-such-project/stages.json          | no-such-project |
      | /p/acme/house-bills/stories/flows.json       | stories         |
      | /p/acme/house-bills/kitchen/flows.json       | kitchen         |

  # Nothing here writes, so nothing here takes a write. The refusal is the method and not the
  # address, and it says which methods the address answers.
  Scenario: The site refuses a method that writes
    Given the site is served on a local address
    When the operator sends the site a write at "/p/acme/house-bills/stages.json"
    Then the site answers 405
    And the site says it answers GET and HEAD

  # The second document. A stage's artifact is json already, and the site hands it back as it is, so
  # the flow map reads the same bytes the session wrote.
  Scenario: A stage's flows are answered from the artifact it carries
    Given the "discovery" design stage is written and approved
    And the operator writes the "stories" design stage with the artifact:
      """
      {"screens": [{"id": "bills", "kind": "web"}]}
      """
    And the site is served on a local address
    When the operator reads the site at "/p/acme/house-bills/stories/flows.json"
    Then the site answers 200
    And the site answers json
    And the answer is the artifact meaning:
      """
      {"screens": [{"id": "bills", "kind": "web"}]}
      """

  # The port is the whole system. So the default binds the machine it runs on, and the compose stack
  # publishes the host side on loopback alone.
  Scenario: The site is served on loopback and published on loopback
    Then the site's default address is "127.0.0.1:50052"
    And the compose stack publishes the site as "127.0.0.1:50052:50052"
    And the compose stack tells the control plane to bind ":50052"

  # The page the operator opens. Two stages are agreed, and then one of them is written again, so the
  # word it carried no longer stands. The menu says which stage is which, and it says it without
  # anybody comparing two version numbers by hand.
  Scenario: The menu shows an approved stage and a stage changed since its approval
    Given the "discovery" design stage is written and approved
    And the "stories" design stage is written and approved
    And the operator writes the "stories" design stage as "four bills, and three of them move"
    And the site is served on a local address
    When the operator reads the site at "/p/acme/house-bills/"
    Then the site answers 200
    And the page names its stylesheet and its script
    When the operator reads the site at "/assets/site.js"
    Then the site answers 200
    When the operator reads the site at "/p/acme/house-bills/stages.json"
    Then the menu drawn from that answer reads "approved" for the "discovery" stage
    And the menu drawn from that answer reads "changed since approval" for the "stories" stage
    And the menu drawn from that answer reads "not written" for the "architecture" stage
    And the menu drawn from that answer lists Design and then the six stages in order

  # A stage body is markdown, and an operator reads a document rather than the marks that make one.
  # The same reading is what makes a body safe to open. A session writes the text, the operator reads
  # the text, and nothing written into a body runs in the operator's browser.
  Scenario: A script written into a stage body is shown and never run
    Given the operator writes the "discovery" design stage as:
      """
      # Four bills

      Two of them move every month.

      - the rent moves
      - the water moves

      ```go
      fmt.Println("the rent")
      ```

      <script>alert("the rent")</script>

      <img src="x" onerror="alert('the water')">
      """
    And the site is served on a local address
    When the operator reads the site at "/p/acme/house-bills/stages.json"
    Then the site answers 200
    And the "discovery" stage reads as a document, with its heading, its list and its code
    And the "discovery" stage runs nothing in the operator's browser
    And the page shows the operator that document

  # A data model and an architecture carry a picture, and step 3 left that picture on the page as the
  # text that describes it. The library that draws it is served by the site itself, so the operator
  # reads the architecture as a diagram on a machine with no network, and the design of a private
  # project never leaves the machine it is written on.
  Scenario: A diagram in a stage is drawn by a library the site serves itself
    Given every design stage is written and approved
    And the site is served on a local address
    When the operator reads the site at "/p/acme/house-bills/stages.json"
    Then the "architecture" stage arrives as a diagram ready to draw
    When the operator reads the site at "/p/acme/house-bills/"
    Then the page loads the drawing library from the site itself
    When the operator reads the drawing library the page names
    Then the site answers 200
    And the site answers a script
    And no file the site hands the operator reaches an address off the machine
