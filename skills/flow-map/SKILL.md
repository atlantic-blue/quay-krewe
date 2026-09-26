# flow-map: the screens of a project, as a page a person can play

The mockups stage is approved by playing each story. This skill is the page that plays it.

You write one data file. The page draws it. The page holds no colour and no font of its own,
so every value a screen is drawn in comes from the project.

## What you write

Write `flows.json` and nothing else. `schema.json` is beside this file. Read it before you
write: it holds every field and every allowed value.

The file has four parts:

- `readAt`, the commit and the date you read the screens at.
- `screens`, each screen, under the name a story calls it by.
- `stories`, what a person does, as a walk over the screens.
- `dataModel`, where the data lives. It is optional, and the Data view is hidden without it.

## A screen is a document

Each screen carries `html`. The field holds the markup of the body of one screen. You write
it, and the page composes it into a document of its own.

Write real markup with real words. A screen an operator would be glad to receive is the work.

Each screen names a surface, `mobile` or `web`. Write the css at the size of that surface:
390 by 844 on a phone, 1280 by 800 in a browser. The page scales the drawing to the map.

A `<style>` element in a screen reaches that screen only, because each screen is drawn in a
document of its own. Put the rules of one screen inside it.

## Two attributes carry the contract

A part names the component it stands for with `data-component`, as in
`data-component="Button"`. The session that builds the screen reads that name. A part a
person can press names one on itself. Every visible part sits under one, so a card names
itself once and the words inside it need no name.

A part that opens another screen carries `data-to`, holding the name of that screen. The
Prototype view follows it when a person presses the part.

## Colours, fonts and marks

Take every value from the approved design_system stage. The check refuses a colour or a font
that stage does not name. Write `var(--t-colour-primary)` and `var(--t-font-sans)`, which
the page writes out of the tokens of that stage.

There is no network. A screen reaches no address at all, and a request from one fails. So a
font comes from an asset of the design system: write `font-family: var(--t-font-display)`,
and the page writes the `@font-face` rule. An image is `<img src="asset:mark" alt="">`, or
`url(asset:mark)` in a rule.

A small mark is better written as an inline `<svg>`. It needs no asset, and an asset costs
bytes in every screen that draws it.

A screen is drawn and it never runs. A `<script>`, an attribute whose name starts with `on`,
and every other address are all refused.

## The worked example

`example/design-system.json` and `example/flows.json` sit beside this file. Copy them. The
example holds one design system with a font file and a mark, and two screens, one on a phone
and one in a browser.

## Save it to the stage

    krewe stage set <address> mockups --file mockups.md --artifact flows.json

The artifact is the data. Nothing else is written, and nothing is published.

## Look at it

    krewe design open <address>

Choose the stage. Play every story once. Press each part that carries `data-to`. Read the
Gaps view. Then the operator approves the stage.
