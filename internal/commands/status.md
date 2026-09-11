<!-- written by krewe <version> -->
---
description: read one project and print the state of its path in one readout
---

Read the state of one project in Quay Krewe.

Run the commands below. Then print one readout.

This command writes nothing. It starts no session. It approves nothing.

## 1. Ask for the address

Ask the operator for the address. An address reads `<workspace>/<project>`.

## 2. Read the project

Run these five commands. Each one reads. None of them writes.

    krewe path cap <workspace>/<project>
    krewe path <workspace>/<project>
    krewe design <workspace>/<project>
    krewe sessions <workspace>/<project>
    krewe trust <workspace>/<project>

Take every number in the readout from what these commands printed.
Do not count the steps. Do not add a number of your own.

A command can refuse. A control plane that does not answer refuses every command.
Show the refusal to the operator. Then stop. Print no readout.
Never print a number that no command gave you.

## 3. Stop when the project has no path

Read what `krewe path <workspace>/<project>` says.

It can say the project holds no feature. It can say a feature holds no path.
Then print one line. Say the project has no path yet. Name `/krewe:design`.

Print no readout after that line. A project with no path has no state to print.

## 4. Read what waits on the operator

The path listing carries one row for each step. Each row carries a state and a verdict.
Read the rows. Do not read a state that no row carries.

A step in state taken holds a session. Read what that session wrote about it.

    krewe step restatement <workspace>/<project> <feature>.<number>

The answer says `approval: ` and a word. A restatement that is not approved waits on the operator.

These wait on the operator:

1. A step in state taken whose restatement is not approved.
2. A step in state taken whose verdict reads passing, at trust level 0.
3. A step in state taken whose verdict reads failing.
4. A design that `krewe design <workspace>/<project>` reads back as not approved.

Nothing there is nothing, and it is not a count of zero. Say that no step waits.

## 5. Print the readout

Print four parts, in this order. The first two say what to do today.

**In flight.** `krewe path cap` printed the cap and the count in flight. Print both numbers.

**Waits on you.** Print one line for each step of step 4 above.
Give the step as `<feature>.<number>`, then its title, then why it waits.

**Next step.** Run this command once for each open feature. It writes nothing.

    krewe path <workspace>/<project> <feature>

The path of one feature ends with a line reading `next: step <number>`.
Print that line for each feature. Print nothing of your own in place of it.

**Trust.** `krewe trust` printed the level, the run of agreements and the threshold.
Print the level and the run.

## 6. Name the command to type

Print one command under the readout. The operator reads one line and acts.

Name the step that waits, or the next step, and print the command that shows it.

    krewe step show <workspace>/<project> <feature>.<number>

Print `/krewe:design` instead when the design reads back as not approved.

## What this command does not do

It writes nothing. It takes no step. It starts no session. It raises no trust level.

The operator acts on what the readout says. This command reads and prints.
