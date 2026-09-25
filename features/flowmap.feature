Feature: The flow map plays a project's stories

  A project is designed in six stages, and the mockups stage is the one an operator approves by
  playing each story. The flow map is the page that plays it. It is a skill rather than a command,
  in `skills/flow-map`: one page, and a `flows.json` a session writes for the project it is
  designing.

  Two things about it are contracts rather than taste. A project has more than one surface, so a
  screen says whether a person looks at it on a phone or in a browser, and the page draws each one
  in the frame that surface asks for. And the page holds no colour and no font of its own: every
  value it paints a screen with comes from the design system the operator approved, which the page
  reads beside the screens. A page with a palette in it would show every project in the same
  colours, and an operator would approve mockups of a product that does not look like that.

  A session writes each screen as the markup of its body. The page composes that markup into a
  document of its own and gives it to a frame, so the operator reads the product rather than a
  drawing of it, and the markup cannot run, cannot call an address, and cannot touch another
  screen while it is read.

  These scenarios run the page's own renderer over the skill's fixture, outside a browser, so what
  is read here is the markup an operator sees.

  Background:
    Given the flow map, the fixture that holds both surfaces, and the design system of the project

  Scenario: The flow map draws a web story and a mobile story in their own frames
    When the operator plays the story "An operator signs in on the web"
    Then the screen is drawn in a browser frame
    And the address bar reads "/sign-in"
    When the operator plays the story "A person logs a tide on the phone"
    Then the screen is drawn in a phone frame

  # The step this feature exists for. The markup was stored before anything read it, so the page
  # carries what an operator opens: the words the session wrote, and none of its reach.
  Scenario: A screen written in html is drawn inside a frame that runs no script of its own
    Given a screen a session wrote as markup, holding a script and an event attribute
    When the operator opens that screen
    Then the operator reads the words the session wrote
    And the screen is drawn at the size of the surface it was written at
    And nothing the session wrote can run
    And the screen can reach no address, and no other screen

  Scenario: A screen is painted in the project's values and in nothing else
    When the operator opens a screen a session wrote as markup
    Then every colour it is drawn in is one the design system names
    And the page put no colour and no font of its own into it
