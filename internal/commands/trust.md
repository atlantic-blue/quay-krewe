<!-- written by krewe <version> -->
---
description: read the trust level of a project, and raise it when the operator says yes
---

Read where the word done sits on one project in Quay Krewe.

Run the read below. Print what it says. Ask for a yes only where an offer stands.

## 1. Ask for the address

Ask the operator for the address. An address reads `<workspace>/<project>`.

## 2. Read the record

Run this command. It reads the record. It writes nothing.

    krewe trust <workspace>/<project>

The output carries the level, the run of agreements, the threshold and the whole record.
Take every number from that output. Do not count an agreement yourself.

A command can refuse. A control plane that does not answer refuses every command.
Show the refusal to the operator. Then stop. Ask nothing.

## 3. Stop when the project has no design

The output can say that the project holds no design yet.
Then print one line. Say the project has no design yet. Say that the level is 0.
Name `/krewe:design` in that line.

Print no ladder after it. A project with no design took no step, so it earned nothing.

## 4. Print the ladder

Print three parts, in this order. Take each number from the output of step 2.

**The level.** Print the level. Print what each level does, one line for each.
Level 0: Krewe checks a step, and the operator says done.
Level 1: Krewe closes a step that its own check passed.

**The run.** Print the run of agreements in a row. Print the threshold beside it.

**The record.** Print the count of agreements. Print the count of disagreements.

## 5. No offer: say what the offer needs, and ask nothing

The output prints a standing offer under the record. Read whether it prints one.

Do this where the output prints no offer. Subtract the run from the threshold.
Print that answer. Say that the offer needs this many more agreements in a row.

Then stop. Ask no question here. The control plane refuses a raise that no offer stands behind.
An agreement comes from the next step the operator finishes.

## 6. An offer stands: say what the raise hands over, then ask for a yes

Do this only where the output prints a standing offer.

Tell the operator what the raise changes. Say these three things:

1. Krewe closes a step that its own check passed, and says so on the check.
   The operator stops saying the word done on this project.
2. A failing check closes nothing. That holds at every level.
3. The way back down is `krewe step reopen <workspace>/<project> <feature>.<number>`.
   The operator types it on a step that Krewe closed wrongly. It drops one level.

Then ask one question. Does Krewe get the word done on this project from now on?

Run this command only after the operator says yes.

    krewe trust raise <workspace>/<project>

Run nothing on a no. Tell the operator that the level stays where it was.
The offer stands. The operator answers it later.

## 7. Read the level back

Read the record again after a raise. The operator reads what the raise wrote.

    krewe trust <workspace>/<project>

Print the level that this output says. Print no level of your own.

## What this command does not do

It never lowers a level. Only the operator lowers one, on a step that Krewe closed wrongly.

It runs the two commands above and no other command.
It takes no step. It starts no session. It writes no design and no path.
