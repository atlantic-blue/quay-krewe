# design: the design of a project, before anybody builds it

You write the design. The operator reads it and approves it. Then another session builds each step,
and that session was not in this conversation.

Every command here takes an address, written as `<workspace>/<project>`.

## 1. Read where the project stands

    krewe stage show <workspace>/<project>

Six stages hold the design, in order: discovery, stories, design_system, mockups, data_model,
architecture. Your ask names one. Write that one, and no other. A stage is refused while the stage
before it carries no approval.

## 2. Read the code, and keep the findings

Read every stage that carries the operator's word already. Read the project's context in your memory
file. Read the code itself, not your memory of it.

The reading is yours. The document is the operator's. So it carries what you found, and never the
route you took. Write no file path, no line number, no name of a function and no count, unless one
decision rests on that exact fact. Then write that fact in one sentence, beside that decision.

Write no list of what you read. Write no list of what you did not read.

## 3. Write one stage as a brief

The operator reads each stage alone and decides on it alone. A stage is a brief from a team lead:
short, and every part of it is a thing to agree with or change. Use these seven headings, in this
order, each written as `## <heading>`.

    Goal                        one sentence, what this stage is for
    Direction                   what you recommend and why, two or three short paragraphs
    Assumptions                 one line each, each ending "correct me or I proceed"
    Decisions for the operator  one line each, each carrying your recommendation
    Done when                   one line each, and each one testable
    Not doing                   one line each, with the reason on the same line
    Open questions              only the ones a guess is not safe for

One page of prose, and nothing longer. A body over 6,000 characters is refused, so a stage that will
not fit is a stage still carrying its research. Write no confidence percentage and no section for
one. Where you are unsure, say so on the line of the decision it affects.

    krewe stage set <workspace>/<project> <stage> --file <path>

The data model and the architecture each carry a mermaid diagram beside the prose. The design system
and the mockups each carry a json artifact, written with `--artifact <path>`, with the page's address
given by `--url <address>`. Read `skills/flow-map/SKILL.md` before the mockups.

## 4. Write the design, once all six stages carry the word

Read all six. They are what the operator agreed to, and the design says nothing more. Write it with
`krewe design set --file <path>`.

Then narrow the project into parts. Add one with `krewe feature add "<title>"`, and say what it
narrows to with `krewe feature intention <feature> "<text>"`.

Write the contracts with `krewe design contracts --file <path>`. A contract says what one thing must
do, and names the stage it came from.

## 5. Write the path of each part

`path.md` beside this brief says how a path document is written, what the seven labels of a step are,
the rules one step holds to, and the six parts of the restatement a session writes before it builds.
Read it when you write a path. The seven headings above bind a stage and not a path.

## 6. Never approve anything

Never approve the design, and never approve a stage. Only the operator approves. You write both, and
you agree to neither.
