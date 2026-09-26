Feature: A mockup is refused unless every part names its component

  The mockups stage carries a flows.json, and an operator approves it by playing each story. A
  session then builds the screens on it. That session has to read which library component each
  part stands for, so a part that names none is a picture nobody can build from.

  The fault used to surface at the end, when the building session reached a rectangle labelled
  "Sign in" and had to guess. So the artifact is read when it is written. A mockup a session could
  not build from never reaches the operator to approve.

  A screen is written as the markup of its body. The component name sits on the markup, as
  data-component. A part that can be pressed names one on itself. Every visible part sits under
  one. So a card names itself once, and the words inside it need no name of their own.

  A screen used to be written as a list of shapes as well. That form is gone, and a session still
  carrying it is told the field to write instead rather than left to read a schema fault.

  A screen is drawn in the colours and the fonts of the design_system stage, which the operator
  approved before the mockups could be written. The stylesheet of a screen, the style of one part and
  the paint of a mark are all read, and a value that stage does not name is a second design system
  nobody agreed to, so it is refused. A font reaches a screen as a token, and the file it draws from
  travels in the design system.

  A design system that names no colour and no font refuses the mockups write, and the refusal says to
  write the tokens of that stage first. Nothing else holds the screens to anything.

  A screen is drawn and it is never run. A script, an attribute whose name starts with "on", and any
  address other than an asset of the design system or a part of the screen itself are all refused.
  The operator plays the mockup on the machine in front of them, and the fonts and the images of the
  project are the ones that operator approved.

  The other five stages carry whatever json they carry. Nothing here reaches them.

  Background:
    Given a running control plane
    And a workspace named "acme"
    And a project named "house-bills"
    And the stages up to the design system are approved, naming the project's colours and fonts

  # There is one way to write a screen. A session that writes the old one is told which field to
  # write instead, in the same sentence, so it has work to do rather than a fault to decode.
  Scenario: A mockup written as a list of shapes is refused and named the field to write instead
    When the operator writes the mockups stage with a screen written as a list of shapes
    Then the control plane refuses it as invalid
    And the refusal suggests "sign-in"
    And the refusal suggests "html"
    And the project holds no mockups stage

  # The refusal this whole feature exists for. It names the screen and the part, because a mockup
  # runs to dozens of screens and an operator told only that a component is missing has to read the
  # whole file to find out where.
  Scenario: A screen whose words sit outside every named component is refused
    When the operator writes the mockups stage with words outside every named component
    Then the control plane refuses it as invalid
    And the refusal suggests "dashboard"
    And the refusal suggests "This week"
    And the refusal suggests "data-component"
    And the project holds no mockups stage

  # An operator plays a mockup on the machine in front of them, and often with no network. A screen
  # that reaches out for a font draws one thing on one machine and another thing on the next, and it
  # tells the host it was opened. Every font and every image of the project sits in the design system
  # the operator approved, so a screen needs no address of its own.
  Scenario: A screen that loads a font from an address outside the page is refused
    When the operator writes the mockups stage with a screen loading a font from the network
    Then the control plane refuses it as invalid
    And the refusal suggests "dashboard"
    And the refusal suggests "fonts.googleapis.com"
    And the refusal suggests "design_system"
    And the project holds no mockups stage

  Scenario: A mockup that names a component on every part goes in
    When the operator writes the mockups stage with a component on every part
    Then the mockups stage carries the artifact it was given

  # The backstop. The schema is the file the skill tells a session to write from, so a document that
  # is json and is not a flows.json is refused against the same file rather than kept.
  Scenario: An artifact that is not a flows file is refused against the schema
    When the operator writes the mockups stage with the artifact:
      """
      {"screens": {}}
      """
    Then the control plane refuses it as invalid
    And the refusal suggests "schema"

  # The tokens are what every screen is drawn in, so one colour outside the design system is wrong
  # on every screen of the mockup.
  Scenario: A mockup drawn in a colour the design system does not name is refused
    When the operator writes the mockups stage drawn in the colour "#ff0000"
    Then the control plane refuses it as invalid
    And the refusal suggests "#ff0000"
    And the refusal suggests "design_system"

  Scenario: A mockup drawn in a font the design system does not name is refused
    When the operator writes the mockups stage drawn in the font "Comic Sans MS, cursive"
    Then the control plane refuses it as invalid
    And the refusal suggests "Comic Sans MS"

  # A screen is written as markup now, so a colour arrives in the stylesheet of that screen, in the
  # style of one part, or in the paint of a mark. Every colour an operator sees in a mockup is a
  # colour that operator already approved, so a screen cannot carry a second design system.
  Scenario: A colour written into the markup of a screen is refused
    When the operator writes the mockups stage with the colour "#ff0000" in the markup of a screen
    Then the control plane refuses it as invalid
    And the refusal suggests "dashboard"
    And the refusal suggests "#ff0000"
    And the refusal suggests "design_system"
    And the project holds no mockups stage

  # A design system written as prose alone names nothing. That write used to go through carrying a
  # warning, and a warning holds nothing: the screens were then drawn in whatever the session chose.
  # A project whose design system names nothing cannot write a mockup at all.
  Scenario: A mockup is refused while the design system names no tokens
    Given the design system is approved naming nothing
    When the operator writes the mockups stage with a component on every part
    Then the control plane refuses it as invalid
    And the refusal suggests "design_system"
    And the refusal suggests "tokens"
    And the project holds no mockups stage

  # Prose first and the page after is how a stage gets written. A mockups stage with nothing to read
  # has nothing to refuse.
  Scenario: A mockups stage carrying prose alone is not refused
    When the operator writes the "mockups" design stage as "the screens, in words"
    And the operator reads the project's design stages
    Then the "mockups" design stage reads "the screens, in words"

  # The check belongs to the mockups and to nothing else. A check that reached the other five would
  # refuse the discovery stage for not being a flows.json.
  Scenario: A stage that is not the mockups carries any json it likes
    When the operator writes the "discovery" design stage with the artifact:
      """
      {"asked": ["when does it move"]}
      """
    And the operator reads the project's design stages
    Then the "discovery" design stage carries an artifact meaning:
      """
      {"asked":["when does it move"]}
      """
