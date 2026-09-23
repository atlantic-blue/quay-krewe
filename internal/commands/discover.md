<!-- written by krewe <version> -->
---
description: read a repository that already exists, and write the discovery stage from what it holds
---

Discover one project that already has a repository.

A session in a sandbox reads the repository. That session writes the discovery stage.
Do not read the repository here. Do not write the stage here.

## 1. Read where the project stands

Ask the operator for the address. An address reads `<workspace>/<project>`.

    krewe stage show <workspace>/<project>

Show the six stages to the operator. Do not shorten the listing.

The discovery stage carries the operator's word already when the listing says approved.
Ask the operator whether to read the repository again. Stop here when the answer is no.

## 2. Read which repository the project names

    krewe project repository show <workspace>/<project>

A project that names no repository has nothing to discover. Say that to the operator.
Then stop. Name `/krewe:init`.

## 3. Make sure the session holds the discover skill

A session holds the skills of its workspace. The skill named `discover` says how to read a
repository and what to write down. Read what the workspace holds now.

    krewe skill list <workspace>

Go to step 4 when the answer names `discover`.

Show the operator this command when the answer does not name `discover`. Ask for a yes.
Then run it.

    krewe skill attach <workspace> discover

A skill attaches to a workspace and not to one session. Every later session of that workspace
holds it. Say that to the operator before you ask for the yes.

A skill reaches a sandbox when the system builds the sandbox. Run this command before the
dispatch. A session that runs already does not hold a skill attached after it started.

## 4. Dispatch the session

Show the operator the command. Ask for a yes. Then run it.

    krewe exec --dispatch <workspace>/<project> "<what the operator wants read>"

Put the words of the operator in the text. Put nothing else there.
The skill tells the session what to write down. Do not repeat the skill here.

Keep the session identifier that the command prints.

A dispatch can fail. Show the refusal to the operator. Then stop. Approve nothing.

## 5. Read the discovery back

The session runs in the system. Read whether it still runs.

    krewe sessions <workspace>/<project>

Wait for the session to stop. Then read what it came back with.

    krewe answer <session>

Then read the six stages again.

    krewe stage show <workspace>/<project>

The listing says written when the session wrote the stage. Then bring the two documents out
of the project's volume.

    krewe volume cp krewe://<workspace>/<project>/discovery.md .
    krewe volume cp krewe://<workspace>/<project>/flows.json .

Print the discovery document whole. Do not write a summary. Do not print the first list only.
The operator approves the words that the operator reads.

Say how many screens the flows.json holds. Name each one with the file it came from.

The listing can still say empty. Then tell the operator that the session wrote no discovery.
Give the session identifier. Do not write a discovery here.

## 6. Ask for a yes, then approve

Ask the operator one question. Is this what the repository holds?

Run this command only after the operator says yes.

    krewe stage approve <workspace>/<project> discovery

Run nothing on a no. The discovery stays unapproved. Tell the operator that it stays unapproved.
Ask the operator what to correct. Then go back to step 4 with that correction.

## What this command does not do

It writes no discovery document. It writes no design. It writes no path.

A session in a sandbox writes the discovery. The discover skill of that session says what to
write down. The stage after discovery is stories, and `/krewe:design` carries that one.
