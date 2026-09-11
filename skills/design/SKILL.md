# design: the design of a project, before anybody builds it

You write the design. The operator reads it and approves it. Then another session builds each step,
and that session was not in this conversation.

Every command here takes an address, written as `<workspace>/<project>`. Give the address when krewe
answers that you are nowhere.

## 1. Read, then say what you read

Read the brief and the design so far with `krewe design`. Read the project's context in your memory
file. Read the repository the project names. Read the code itself, not your memory of it.

Name in the design what you read, file by file. A design written from a belief about a repository
disagrees with the repository. The operator must see which one you did.

## 2. Write the design

Write one document. Write it with `krewe design set --file <path>`.

Then narrow the project into parts. Add one part with `krewe feature add "<title>"`. Say what that
part narrows to with `krewe feature intention <feature> "<text>"`.

Write the contracts document with `krewe design contracts --file <path>`. A contract says what one
thing must do. Every step names the contracts it builds, and the scope of each one.

## 3. Write the path of each part

Write one path document for each part. Write it with `krewe path set <feature> --file <path>`.

A milestone heading reads `# 4. <title>`. A step heading reads `## 1. <title>`. Step numbers are
unique across the whole document. Two steps under different milestones cannot share a number.

Put these seven labels under a step heading. Each label sits alone on its own line.

    What changes and why
    What this touches
    What proves it
    The scenario that proves it
    The contracts it builds
    The scope of each contract
    After

`After` holds the number of the step this step waits for, or `0`. Each scope line reads
`<identifier>: <sentence>`.

## 4. The rules for one step

- One step is one intention, and one change a person can review. A title that needs the word "and"
  is two steps.
- Write each step for a person who was not in this conversation. That person must build the step
  without a question.
- `What proves it` states the value the step delivers. `The scenario that proves it` names one
  scenario, and that scenario describes that value. A step that names no scenario cannot be
  checked, and `krewe step check <feature>.<number>` refuses it.
- `What this touches` names every file the step writes, one file for each line. It names no file the
  step only reads. Krewe compares those lines, and refuses a step that shares a file with a step
  that runs now. A step that names no file collides with nothing, so two sessions can write over
  each other.

## 5. The six parts of a restatement

A session writes six parts before it builds the step it took. Write all six:

1. What this step changes, in your own words.
2. What this step will not touch.
3. What you assumed, that the step did not say.
4. What you do not know.
5. The scenario you will write, by name, and the value that scenario describes.
6. How sure you are, as a percentage, and what lowers that.

## 6. Never approve the design

Only the operator approves. You write the design, and you do not agree to it.
