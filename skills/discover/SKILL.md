# discover: what a repository already holds, written down

You read a repository that exists. You write the discovery stage of its project. The operator reads
that stage and approves it. Every later stage builds on what you wrote. A screen you leave out is a
screen somebody designs a second time.

Every command here takes an address, written as `<workspace>/<project>`.

## 1. Read the repository, not your memory of it

Read the files. Write down the file each thing came from. A discovery written from a belief about a
repository disagrees with the repository.

## 2. Write six lists

Write one document. Name the file each item came from.

1. Routes and screens. One line for each route: the path, the screen it draws, and the file.
2. Components. The parts a screen is built from, and the file of each part.
3. Tokens. The colours, the fonts, the spacing and the radius, and the file that holds them.
4. Data entities. The things the product keeps, and the file of each one.
5. Patterns. The rules the code follows today. How it reads data. How it answers a refusal. How it
   names a test.
6. Test tiers. Each kind of test, the command that runs it, and what it covers.

End with one line for what you did not read, and why.

`example/discovery.md` beside this brief is a whole document of the six lists. Copy its shape.

## 3. Write the draft flows.json

Write a second file. It holds the screens that exist today, and nothing else.

    {
      "readAt": {"repository": "<owner>/<name>", "commit": "<sha>", "date": "<yyyy-mm-dd>"},
      "tokens": {"colour": {}, "font": {}, "space": {}, "radius": {}},
      "screens": {
        "<id>": {
          "name": "<what a person calls it>",
          "surface": "web",
          "status": "built",
          "route": "/<path>",
          "source": "<the file it came from>",
          "el": [{"t": "h", "v": "<the words on the screen>", "component": "Heading"}]
        }
      },
      "stories": [],
      "dataModel": []
    }

Four rules hold for every screen.

- The status reads `built`. Discovery writes down what exists. A screen nobody built is not yours.
- The surface reads `web` or `mobile`. Each one is drawn in its own frame.
- The source names the file you read the screen from.
- Each colour, font, space and radius comes from `tokens`. Write no colour beside a shape.

`example/flows.json` beside this brief is a whole file for a repository of three routes. Copy it.

## 4. Write the stage, then stop

    krewe stage set <workspace>/<project> discovery --file discovery.md --artifact flows.json

Put both files where the operator can read them.

    krewe volume cp discovery.md krewe://<workspace>/<project>
    krewe volume cp flows.json krewe://<workspace>/<project>

## 5. Never approve the stage

Only the operator approves. You write the discovery, and you do not agree to it. A session is
refused `krewe stage approve`.
