# The path of one part, and the step that builds it

Read this when you write a path document. The brief beside it says how a stage is written; this says
how the work under an approved design is written down.

## Write one path document for each part

    krewe path set <feature> --file <path>

A milestone heading reads `# 4. <title>`. A step heading reads `## 1. <title>`. Step numbers are
unique across the whole document.

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

The seven headings a stage holds do not bind a path, and neither does the length of a stage. A path
is read by the session that builds one step of it, and it says everything that session needs.

## The rules for one step

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

## The six parts of a restatement

A session writes six parts before it builds the step it took. Write all six:

1. What this step changes, in your own words.
2. What this step will not touch.
3. What you assumed, that the step did not say.
4. What you do not know.
5. The scenario you will write, by name, and the value that scenario describes.
6. How sure you are, as a percentage, and what lowers that.
