# What acme/house-bills holds today

Read at commit 4c1f9a2 on 2026-09-08.

## Routes and screens

- `/` draws Every bill, a list of the bills of one household. `app/routes/bills.tsx`
- `/bills/:id` draws One bill, which is the form that writes one. `app/routes/bill.tsx`
- `/settings` draws Settings, which is who else sees the bills. `app/routes/settings.tsx`

## Components

- `BillRow`, one bill in the list, with its amount and the day it is due. `app/ui/BillRow.tsx`
- `MoneyField`, an amount, kept in pence. `app/ui/MoneyField.tsx`
- `DateField`, a day, with no time beside it. `app/ui/DateField.tsx`
- `Button`, `Link`, `Heading` and `TextField`. `app/ui/primitives.tsx`

## Tokens

- Colour: ink `#141414`, paper `#faf9f7`, accent `#2f6f5e`, quiet `#6b6b6b`. `app/styles/tokens.css`
- Font: body `Inter, system-ui, sans-serif`, figure `InterTabular, monospace`. `app/styles/tokens.css`
- Space: row `8px`, block `24px`. Radius: card `12px`. `app/styles/tokens.css`
- Nothing else in the repository holds a colour. Every part reads a token.

## Data entities

- `Bill`: an amount in pence, a day, a name, and the household it belongs to. `app/data/bill.ts`
- `Household`: a name, and the people who may read its bills. `app/data/household.ts`

## Patterns

- A route reads its data in a loader and never in the component. `app/routes/bills.tsx`
- A refusal comes back as a problem document, and the route draws the title of it. `app/data/fetch.ts`
- A test is named for the behaviour it holds, never for the function. `app/routes/bills.test.tsx`

## Test tiers

- Unit, `npm test`, over `app/data` and `app/ui`. It runs in about nine seconds.
- Browser, `npm run e2e`, over the three routes. It runs in about two minutes.

## What I did not read

The deploy pipeline under `.github/workflows`. It ships the app and draws no screen.
