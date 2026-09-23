<!-- written by krewe <version> -->
---
description: walk the six design stages in order, then design the project from the six the operator approved
---

Design one project in Quay Krewe.

A project is designed in six stages, in this order: discovery, stories, design_system, mockups,
data_model, architecture. This command designs one of them in one run. The order lives in the
control plane, and this file reads it rather than repeating it.

A session in a sandbox writes each stage. It writes the design too. Do not write a stage here. Do
not write the design here.

## 1. Read where the project stands

Ask the operator for the address. An address reads `<workspace>/<project>`.

    krewe stage show <workspace>/<project>

Show the six stages to the operator. Do not shorten the listing.

The first stage without the operator's word is the stage this run is about. The line under the
listing names it. Take that name from the listing every time. A stage is refused while the stage
before it carries no word, so a stage you chose here is a stage the system may refuse.

Stop here when the listing names the discovery stage and the listing says empty. Name
`/krewe:discover` to the operator. That command reads the repository and writes the discovery down.

Go to step 5 when the listing says the stage is written. The document is there, and it needs a
reading.

Go to step 7 when the listing says every stage carries the operator's word.

## 2. Ask the questions of that stage

Ask one question at a time. Wait for the answer to each one. Keep the words of the operator. Do not
answer a question for the operator. Ask a question again when the answer is not clear.

Ask these for the stories stage.

1. Who uses this project? What does each person do with it?
2. What must a person do that they cannot do today?
3. What says that one story is finished?

Ask these for the design system stage.

1. What must this project look like? Name a product that feels right.
2. Which colours, type and spacing are fixed already?

Ask these for the mockups stage.

1. Which story does a person walk through first?
2. Which surface draws each screen, web or mobile?

Ask these for the data model stage.

1. What does this project keep? How long does it keep each thing?
2. Which of those things does a person read by key?

Ask these for the architecture stage.

1. Where does this project run?
2. What may it cost while nobody uses it?

## 3. Make sure the session holds the design skill

A session holds the skills of its workspace. The skill named `design` says how to write a stage and
how to write a design. Read what the workspace holds now.

    krewe skill list <workspace>

Go to step 4 when the answer names `design`.

Show the operator this command when the answer does not name `design`. Ask for a yes. Then run it.

    krewe skill attach <workspace> design

A skill attaches to a workspace and not to one session. Every later session of that workspace holds
it. Say that to the operator before you ask for the yes.

A skill reaches a sandbox when the system builds the sandbox. Run this command before the dispatch.
A session that runs already does not hold a skill attached after it started.

## 4. Dispatch the session for that stage

Show the operator the command. Ask for a yes. Then run it.

    krewe exec --dispatch <workspace>/<project> "<stage>: <the answers the operator gave>"

Put the name the listing gave you first. Put the answers of the operator after it. Put nothing else
there. Dispatch one session, for that stage alone. The skill tells the session how to write a stage.
Do not repeat the skill here.

Keep the session identifier that the command prints.

A dispatch can fail. Show the refusal to the operator. Then stop. Approve nothing.

## 5. Read the stage back

A dispatch in step 4 leaves a session running. Read whether it still runs. Go straight to the volume
when you came here from step 1, because nothing ran in this conversation.

    krewe sessions <workspace>/<project>

Wait for the session to stop. Then read what it came back with.

    krewe answer <session>

Then read the six stages again.

    krewe stage show <workspace>/<project>

The listing says written when the session wrote the stage. Then bring the document out of the
project's volume.

    krewe volume cp krewe://<workspace>/<project>/<stage>.md .

Print the document whole. Do not write a summary. Do not print the first paragraph only. The
operator approves the words that the operator reads.

The mockups stage carries a page as well. Give the operator its address, and ask them to play each
story on it.

The listing can still say empty. Then tell the operator that the session wrote no stage. Give the
session identifier. Do not write a stage here.

## 6. Ask for a yes, then approve that stage

Ask the operator one question. Is this the stage to build the next one on?

Run this command only after the operator says yes.

    krewe stage approve <workspace>/<project> <stage>

Run nothing on a no. The stage stays unapproved. Tell the operator that it stays unapproved. Ask the
operator what to change. Then go back to step 4 with that change.

Say this to the operator after the approval. One stage carries your word now. Run `/krewe:design`
again for the stage after it.

## 7. Design the project from the six stages

Every stage carries the operator's word here. So the design of the whole project is the move that is
left.

Show the operator the command. Ask for a yes. Then run it.

    krewe exec --dispatch <workspace>/<project> "write the design from the six approved stages"

The session reads every stage. It writes the design, the parts of the project, the contracts and the
path of each part. The skill says how. Do not repeat the skill here.

## 8. Read the design back, then approve it

    krewe sessions <workspace>/<project>

Wait for the session to stop. Then read the design.

    krewe design <workspace>/<project>

Print the design body whole. Do not write a summary. A design body can be empty. Then tell the
operator that the session wrote no design. Give the session identifier. Do not write a design body
here.

Ask the operator one question. Is this the design to build from?

Run this command only after the operator says yes.

    krewe design approve <workspace>/<project>

Run nothing on a no. The design stays unapproved. Tell the operator that it stays unapproved. Ask
the operator what to change. Then go back to step 7 with that change.

## What this command does not do

It writes no stage. It writes no design body. It writes no path.

A session in a sandbox writes all three. The design skill of that session says how. The order of the
six stages belongs to the control plane, and this command reads it from the listing.
