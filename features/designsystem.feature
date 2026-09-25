Feature: The design system stage carries its tokens, its files and its stylesheet

  The mockups check holds every screen to the colours and the fonts of the approved design system.
  That stage had no shape of its own, so it carried whatever json a session wrote. A design system
  written as prose named nothing at all, and the write then answered that the mockup was kept
  without that check. Nothing held those screens to anything.

  So the stage is read when it is written. It carries its tokens, the font files and the images a
  screen is drawn with, and the base stylesheet every screen is given. The files travel inside the
  stage, because a screen written as markup reaches a font nowhere else.

  A file too big to travel is refused. So is a colour written beside the tokens, because a screen
  drawn in it is drawn in a design system nobody approved. The refusal binds a write, so a design
  system already stored stays as it is.

  Background:
    Given a running control plane
    And a workspace named "acme"
    And a project named "house-bills"
    And the stages before the design system are approved

  # The one this step exists for. The design system the skill ships is the file a session reads to
  # write one, so a stage that keeps it whole is a stage every screen of the project can be drawn
  # from, down to the font file.
  Scenario: A design system carrying its tokens, its font file and its stylesheet is kept
    When the operator writes the design system the flow map skill ships
    Then the design system stage carries the tokens, the font file and the stylesheet it was given
    And the font file the design system carries names a family one of its font tokens names
    And the write says nothing is wrong

  # The cap the stage needs because everything in it travels in one request. A font nobody can send
  # is a font no screen is ever drawn in, and the operator finds that out at the write.
  Scenario: A design system whose font file is over the cap is refused
    When the operator writes the design system with a font file of 600 kibibytes
    Then the control plane refuses it as invalid
    And the refusal suggests "inter-regular"
    And the refusal suggests "512"
    And the project holds no "design_system" design stage

  # The rule the mockups check already holds a screen to, now held at the source. A stylesheet that
  # paints in a colour the tokens do not name is a second design system, in the one file every
  # screen is given.
  Scenario: A design system whose stylesheet is drawn in a colour the tokens do not name is refused
    When the operator writes the design system with the stylesheet colour "#ff0000"
    Then the control plane refuses it as invalid
    And the refusal suggests "#ff0000"
    And the project holds no "design_system" design stage

  # The check belongs to this one stage. A check that reached the other five would refuse the
  # discovery stage for not being a design system.
  Scenario: A stage that is not the design system carries any json it likes
    When the operator writes the "discovery" design stage with the artifact:
      """
      {"asked": ["what does it look like"]}
      """
    And the operator reads the project's design stages
    Then the "discovery" design stage carries an artifact meaning:
      """
      {"asked":["what does it look like"]}
      """
