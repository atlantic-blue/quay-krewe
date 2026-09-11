<!-- written by krewe <version> -->
---
description: ask the design questions, dispatch a session to write the design, and show it back
---

Design one project in Quay Krewe.

Ask the operator the questions below. Then dispatch a session to write the design up.

Do not write the design here. A session in a sandbox writes it. The record keeps it there.

## 1. Read what the project says now

Ask the operator for the address. An address reads `<workspace>/<project>`.

Read the brief and the design so far.

    krewe design <workspace>/<project>

Show the operator what it says. Do not shorten it.

An approved design is settled already. Ask the operator whether to design the project again.
Stop here when the answer is no.

## 2. Ask the design questions

Ask one question at a time. Wait for the answer to each one.

1. What must this project do that it cannot do today?
2. Who uses it? What does each person do with it?
3. What must it not do?
4. What in the repository does this build on?
5. What decides that the work is done?

Keep the words of the operator. Do not answer a question for the operator.
Ask a question again when the answer is not clear.

## 3. Make sure the session holds the design skill

A session holds the skills of its workspace. The skill named `design` says how to write a
design. Read what the workspace holds now.

    krewe skill list <workspace>

Go to step 4 when the answer names `design`.

Show the operator this command when the answer does not name `design`. Ask for a yes.
Then run it.

    krewe skill attach <workspace> design

A skill attaches to a workspace and not to one session. Every later session of that workspace
holds it. Say that to the operator before you ask for the yes.

A skill reaches a sandbox when the system builds the sandbox. Run this command before the
dispatch. A session that runs already does not hold a skill attached after it started.

## 4. Dispatch the session

Show the operator the command. Ask for a yes. Then run it.

    krewe exec --dispatch <workspace>/<project> "<the answers the operator gave>"

Put the answers of the operator in the text. Put nothing else there.
The skill tells the session how to write a design. Do not repeat the skill here.

Keep the session identifier that the command prints.

A dispatch can fail. Show the refusal to the operator. Then stop. Approve nothing.

## 5. Read the design back

The session runs in the system. Read whether it still runs.

    krewe sessions <workspace>/<project>

Read what the session did.

    krewe exec list <session>

Wait for the session to stop. Then read the design.

    krewe design <workspace>/<project>

Print the design body whole. Do not write a summary. Do not print the first paragraph only.
The operator approves the words that the operator reads.

A design body can be empty. Then tell the operator that the session wrote no design.
Give the session identifier. Do not write a design body here.

## 6. Ask for a yes, then approve

Ask the operator one question. Is this the design to build from?

Run this command only after the operator says yes.

    krewe design approve <workspace>/<project>

Run nothing on a no. The design stays unapproved. Tell the operator that it stays unapproved.
Ask the operator what to change. Then go back to step 4 with that change.

## What this command does not do

It writes no design body. It writes no path.

A session in a sandbox writes both. The design skill of that session says how.
