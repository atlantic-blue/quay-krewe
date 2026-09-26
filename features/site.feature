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

  A stage that holds screens carries them as an artifact, and the site plays them. The flow map is
  the page that plays a story, it ships with the skill that writes the screens, and the site serves
  it from the binary at the stage the screens belong to. The page asks the stage for its screens.
  So an operator approves the mockups by playing them, and nothing was published anywhere.

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
      | /p/acme/house-bills/stories/map/             | stories         |
      | /p/acme/house-bills/kitchen/map/             | kitchen         |

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

  # The stage an operator approves by playing it. The screens sit in the database and the page that
  # plays them sits in the binary, so the operator opens the stage and plays a story. Nobody
  # published a page to a web host, and nobody copied a file next to another file.
  Scenario: The mockups stage plays its story from the stored artifact
    Given the stages up to the design system are approved, naming the project's colours and fonts
    And the operator writes the mockups stage with a component on every part
    And the site is served on a local address
    When the operator opens the flow map of the "mockups" stage
    Then the site answers 200
    And the page plays the project's stories
    When the operator reads the screens the page asks for
    Then the site answers 200
    And the screens are the ones the session wrote
    When the operator reads the site at "/p/acme/house-bills/stages.json"
    Then the page opens the flow map on the "mockups" stage
    And the page opens no flow map on the "architecture" stage

  # A person types this address, and a person leaves the last slash off it. The page asks for its
  # screens one step above wherever it is, so at the address without the slash it would ask the
  # project for them and draw nothing. The site sends the operator to the address that plays.
  Scenario: The flow map address without its last slash sends the operator to the one with it
    Given the stages up to the design system are approved, naming the project's colours and fonts
    And the operator writes the mockups stage with a component on every part
    And the site is served on a local address
    When the operator opens the flow map of the "mockups" stage without its last slash
    Then the site sends the operator to the flow map of the "mockups" stage

  # A stage that holds no screens has nothing to play. The address says so, rather than drawing a
  # page that then asks for screens nobody wrote.
  Scenario: A stage that holds no screens has no flow map to open
    Given the "discovery" design stage is written and approved
    And the site is served on a local address
    When the operator opens the flow map of the "discovery" stage
    Then the site answers 404
    And the answer says "carries no artifact"

  # The design system is the stage nobody approves by reading it. A colour is agreed to when a person
  # sees it, so the page draws every token the stage names: the colour itself, a line set in the font,
  # the corner a radius rounds and the room a space leaves, each beside its name and its value. A
  # palette of the page's own would show the operator a design system nobody wrote.
  Scenario: The design system stage draws a swatch for every colour it names
    Given the stages up to the design system are approved, naming the project's colours and fonts
    And the site is served on a local address
    When the operator reads the site at "/p/acme/house-bills/stages.json"
    Then the site answers 200
    And the design system stage draws a swatch for every colour the project names, with its value
    And the design system stage draws every font, radius and space the project names, with its value
    And the design system stage draws no colour and no font the project does not name

  # The screens of a project are drawn in one design system, and the page that draws them has to
  # reach it. So the site answers it beside the design the operator already reads there. A project
  # that wrote no design system is told so in one sentence, because a page that draws nothing and
  # says nothing reads as a broken site.
  Scenario: The site answers the design system a project approved
    Given the stages before the design system are approved
    And the operator writes the design system the flow map skill ships
    And the operator approves the "design_system" design stage
    And a project named "kitchen-shelf" beside it
    And the site is served on a local address
    When the operator reads the site at "/p/acme/house-bills/design-system.json"
    Then the site answers 200
    And the site answers json
    And the design system it answers carries the operator's word, at the version they approved
    And the design system it answers is the one the project's screens are drawn in
    When the operator reads the site at "/p/acme/kitchen-shelf/design-system.json"
    Then the site answers 404
    And the answer says "kitchen-shelf"
    And the answer says "design_system"
