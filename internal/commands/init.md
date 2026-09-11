<!-- written by krewe <version> -->
---
description: start a project, ask what it is for, and set the brief
---

Start one project in Quay Krewe.

Ask the operator three questions. Then run the tool. Do not design the product here.

## Ask these three questions

Ask one question at a time. Wait for the answer to each one.

1. What is this project for? Ask for one paragraph. Keep the words of the operator.
2. Which workspace holds the project? Run `krewe workspace list` first. Show the names.
3. Which repository does the work land in? Ask for the owner and the name.

## Show the three commands, then ask for a yes

Print the three commands. Do not run a command before the operator says yes.

    krewe project create <workspace>/<project>
    krewe project repository <workspace>/<project> <owner>/<repository>
    krewe design brief <workspace>/<project> "<the paragraph the operator wrote>"

Run them in this order. Each command needs the one above it.

The brief is the paragraph the operator wrote. Do not write a brief of your own.
Do not shorten the paragraph. Do not add a sentence to it.

## Print the address

Print `<workspace>/<project>` on a line of its own. The next command needs the address.

Read the brief back with `krewe design <workspace>/<project>`. Show what it says.

## If a command refuses

Show the refusal to the operator. Then ask the question again.

A workspace that is not there is a question for the operator. Do not create a workspace.
A project name that is already taken is a question for the operator. Ask for another name.

## What this command does not do

It writes no design document. It writes no path.

A session in a sandbox writes the design. The record keeps the design there.
