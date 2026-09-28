# Discovery: acme/house-bills

Read at commit 4c1f9a2 on 2026-09-08.

## Goal

Say what house-bills already draws and already keeps, so the five stages after this one design
around it instead of around a blank page.

## Direction

Bills are the whole product. A household reads what it owes on one list, opens one bill to write it,
and says on a third screen who else may read them. Three routes, two entities, and nothing else
behind a flag.

Money and days are what this code is careful about. An amount is pence, held as a whole number. A
day carries no time beside it. Both rules live in the field components every screen reads, so a new
screen that keeps money as a decimal disagrees with every screen there is.

One file, `app/styles/tokens.css`, holds every colour, font, space and radius value, and nothing
else in the repository holds a colour. That is the one path worth writing down, because the design
system stage starts from a real palette here rather than inventing one. Data comes into a screen
through its route and never through the component, and a refusal comes back as a problem document
whose title the route draws.

## Assumptions

- The three routes are the whole product. Correct me or I proceed.
- Pence stays the unit for every amount we add. Correct me or I proceed.
- The browser suite is the only cover a screen gets, so each new screen needs a case in it. Correct
  me or I proceed.

## Decisions for the operator

- Keep the green accent or pick a new one: I recommend keeping it, because one file holds it and
  every screen reads that file.
- A household shares its bills by invitation and by nothing else: I recommend leaving that rule as
  it is in this project.
- The unit test suite runs in about nine seconds and the browser suite in about two minutes: I
  recommend we keep both, and add to the browser suite only for a new screen.

## Done when

- Each route is written down with the screen it draws, and the artifact holds that screen.
- Each entity is written down with what it keeps.
- flows.json holds one screen for each route, each marked built, each naming its surface and its
  file.
- The colour, font, space and radius values are written down for the design system stage to read.

## Not doing

- The deploy pipeline, because it ships the app and draws no screen.
- A fourth screen, because discovery writes down what exists and designs nothing.
- The colours as design tokens, because the operator approves the design system stage first.

## Open questions

- Nothing in the code says what happens to a bill after somebody pays it. Is a paid bill a state we
  have to carry, or does the row simply stay?
