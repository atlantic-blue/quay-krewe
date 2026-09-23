Feature: The flow map plays a project's stories

  A project is designed in six stages, and the mockups stage is the one an operator approves by
  playing each story. The flow map is the page that plays it. It is a skill rather than a command,
  in `skills/flow-map`: one page, and a `flows.json` a session writes for the project it is
  designing.

  Two things about it are contracts rather than taste. A project has more than one surface, so a
  screen says whether a person looks at it on a phone or in a browser, and the page draws each one
  in the frame that surface asks for. And the page holds no colour and no font of its own: every
  value it paints a screen with comes from the tokens of the flows.json it was given, which come in
  turn from the approved design system stage. A page with a palette in it would show every project
  in the same colours, and an operator would approve mockups of a product that does not look like
  that.

  These scenarios run the page's own renderer over the skill's fixture, outside a browser, so what
  is read here is the markup an operator sees.

  Background:
    Given the flow map and the fixture that holds both surfaces

  Scenario: The flow map draws a web story and a mobile story in their own frames
    When the operator plays the story "An operator signs in on the web"
    Then the screen is drawn in a browser frame
    And the address bar reads "/sign-in"
    When the operator plays the story "A person logs a tide on the phone"
    Then the screen is drawn in a phone frame
    And every colour it is drawn in is one the project's tokens name

  Scenario: A screen is painted in the project's values and in nothing else
    When the page's own rules are read
    Then no rule that paints a screen carries a colour or a font of its own
    And no rule outside that block reaches a screen
