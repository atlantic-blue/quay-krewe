# discover: what a repository already holds, written down

You read a repository that exists, and you write the discovery stage of its project. The operator
reads it and approves it, and every later stage builds on it. A screen you leave out is a screen
somebody designs a second time.

Every command here takes an address, written as `<workspace>/<project>`.

## 1. Read the repository, not your memory of it

Read the files: the routes, the components, the tokens, the entities, the patterns the code follows
today, and the test tiers with what each covers. A discovery written from a belief about a repository
disagrees with the repository.

The reading is yours. The document is the operator's. So it carries what you found, and never the
route you took. Write no file path, no line number, no name of a function and no count, unless one
decision rests on that exact fact. Then write that fact in one sentence, beside that decision.

Write no list of what you read. Write no list of what you did not read. The files are in the
artifact, one for each screen, where a later stage can use them.

## 2. Write the stage as a brief

The operator reads the stage alone and decides on it alone. It is a brief from a team lead: short,
and every part of it is a thing to agree with or change. Use these seven headings, in this order,
each written as `## <heading>`.

    Goal                        one sentence, what this stage is for
    Direction                   what you recommend and why, two or three short paragraphs
    Assumptions                 one line each, each ending "correct me or I proceed"
    Decisions for the operator  one line each, each carrying your recommendation
    Done when                   one line each, and each one testable
    Not doing                   one line each, with its reason on the same line
    Open questions              only the ones a guess is not safe for

One page of prose, and nothing longer. A body over 6,000 characters is refused, so a discovery that
will not fit is a discovery still carrying its reading. Write no confidence percentage and no section
for one. Where you are unsure, say so on the line of the decision it affects.

`example/discovery.md` beside this brief is a whole document in this shape. Copy it.

## 3. Write the draft flows.json

Write a second file. It holds the screens that exist today, and nothing else.

    {
      "readAt": {"repository": "<owner>/<name>", "commit": "<sha>", "date": "<yyyy-mm-dd>"},
      "screens": {
        "<id>": {
          "name": "<what a person calls it>",
          "surface": "web",
          "status": "built",
          "route": "/<path>",
          "source": "<the file it came from>",
          "html": "<the markup of the screen>"
        }
      },
      "stories": []
    }

Write the markup of each screen you found. One heading and one list is enough. You are writing down
what is there, not designing it. Name the component each part stands for with `data-component`, and
name the screen a part opens with `data-to`.

Four rules hold for every screen.

- The status reads `built`. Discovery writes down what exists. A screen nobody built is not yours.
- The surface reads `web` or `mobile`. Each is drawn in its own frame.
- The source names the file you read the screen from. This is where a file path belongs.
- Write no colour and no font in the markup. Say in the prose which one file holds the token values.

`example/flows.json` beside this brief is a whole file for a repository of three routes. Copy it.

## 4. Write the stage, then stop

    krewe stage set <workspace>/<project> discovery --file discovery.md --artifact flows.json

Put both files where the operator can read them.

    krewe volume cp discovery.md krewe://<workspace>/<project>
    krewe volume cp flows.json krewe://<workspace>/<project>

## 5. Never approve the stage

Only the operator approves. You write the discovery, and you do not agree to it. A session is
refused `krewe stage approve`.
