# flow-map: the screens of a project, as a page a person can play

The mockups stage is approved by playing each story. This skill is the page that plays it.

You write one data file. The page draws it. Nothing in the page is about one project: every
colour, font, radius and space comes from the data.

## What you write

Write `flows.json` and nothing else. The page is in this skill, and krewe serves it at the stage you
write the screens to, so you never copy it anywhere.

`schema.json` is beside this file. Read it before you write. It holds every field and every
allowed value.

The file has five parts:

- `readAt`, the commit and the date you read the screens at.
- `tokens`, every colour, font, radius and space. The page holds none of its own.
- `screens`, each screen, under the name a story calls it by.
- `stories`, what a person does, as a walk over the screens.
- `dataModel`, where the data lives. It is optional. The Data view is hidden without it.

## Two rules the page holds you to

Each screen names a surface. Write `mobile` or `web`. The page draws a mobile screen in a phone.
It draws a web screen in a browser with an address bar, and the address bar reads the `route`. A
screen that names no surface is drawn in a phone, and the Gaps view reports it.

Each shape names the component it stands for, for example `Button`. A session that builds the
screen reads that name to know which component goes where. The mockups stage refuses a shape that
names no component.

## Tokens

Take the values from the approved design_system stage. Do not invent them here. The mockups stage
refuses a colour or a font that stage does not name.

The page draws with these names: colour `surface`, `surface-low`, `ink`, `muted`, `line`,
`primary`, `on-primary` and `frame`; font `sans` and `mono`; radius `screen`, `control` and
`card`; space `gap` and `pad`. Add more names if you want. The page writes every token it is
given as a custom property.

## Shapes

One shape is one object in a screen's `el` list. The kind goes in `t`:

`h`, `p`, `eyebrow`, `top`, `card`, `btn`, `btn2`, `link`, `links`, `input`, `fields`, `list`,
`rows`, `chips`, `opts`, `tiles`, `stat`, `quote`, `code`, `image`, `table`, `nav`, `dock`,
`spacer`.

Put `to` on a shape to make it open another screen. The Prototype view follows it when a person
presses the shape.

## Save it to the stage

Write the stage with the prose and the screens:

    krewe stage set <address> mockups --file mockups.md --artifact flows.json

The artifact is the data. Nothing else is written, and nothing is published.

## Look at it

Open the project's design page and choose the stage:

    krewe design open <address>

The stage shows the flow map on the screens you wrote. Play every story once. Press each shape that
carries a `to`. Read the Gaps view. Then the operator approves the stage.
