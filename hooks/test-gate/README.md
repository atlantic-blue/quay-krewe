# test-gate

Refuses a session that builds against failing tests when it changes one of them.

The build stage hands a worker a suite that is already red and asks it to make the tests pass. The
shortest way to a green suite is to change the assertion. A session that does it is not dishonest.
From the inside, a failing test looks exactly like a wrong test, and nothing tells the two apart.

The rule was advice for as long as it existed. Advice is what a model weighs against everything else
it was told, and a boundary a session can talk itself past is not a boundary. So this is checked at
the moment the session tries, by a process the session does not control.

## When it is on

From the moment krewe records the run that saw a step's tests fail, for the session holding that
step, and for no other session.

The moment is the whole rule. The same session writes those tests first, so a gate that came on at
the take would refuse that session for doing what the step asked of it. A run that passed saw
nothing fail, and a run that failed with no scenario in it executed nothing, so neither one turns
anything on. The mark goes away when the step is done or stopped.

A session that holds no step is refused nothing here.

## How it is told

The system writes a file into the session's own directory, at `.krewe/building`. The gate reads
whether that file is there.

A file rather than an environment variable, because of when the gate comes on. The red run lands in
the middle of a session's life, and a container that is already running cannot be handed a new
variable. It can be handed a file: the session's directory is a bind mount, so the system writes
into it from outside and the next tool call reads it. The same file is written again on every exec,
out of the record the store holds, so a session whose container was replaced comes back under the
gate.

The mark is looked for in the directory the runtime says the session is working in, and then in each
directory above it. A session clones the repository it works on into its own directory, so most
commands run a level or two below the mark.

A session may read the mark. It may not write it, and it may not take it away, and nor may it take
away the directory holding it. A boundary a session can lift is advice with extra steps.

`KREWE_BUILDING` is still read, for a worker the system starts under the boundary. A command that
sets or clears the variable is refused, whether the gate is on or off, for the same reason.

## How it reads a command

The program decides, in four classes, and this is the part worth arguing with.

A program that only **reads** is never asked about its paths. `cat`, `grep`, `go`, `make` and the rest
of the list in `command.go`. That is what keeps `go test ./features/` and `make features` working
while the gate is on.

A program that **writes** has every path it was handed read as a path, so a bare word is a directory.
`rm`, `mv`, `cp`, `ln`, `tee`, `truncate`, `patch`, `dd` and the others. Then `sed`, `perl`, `awk`,
`gofmt` and `prettier`, once a flag tells them to write in place. The short spelling, the joined one
and the long one. The verbs of version control that put another copy in the working tree: `git checkout`,
`git restore`, `git stash`, `git clean`, `git rm` and `git mv`. And `find`, once it carries `-delete`
or an `-exec`.

A program that **applies** content from somewhere the line does not show has where it lands read as a
directory taken whole. `tar` extracting, `unzip` and `patch`. Then the git verbs that write the tree
out of another commit or a patch: `apply`, `am`, `cherry-pick`, `revert`, `merge`, `pull`, `rebase`
and `reset --hard`.

Anything **else** is unknown, and only the words that look like a path are read out of it. That is
what stops `python3 -c "open('a_test.go','w')"` and lets `make features` through, and it is what a
program nobody thought of falls into.

Around all three, four readings. A redirect target is a file the line writes. A shell handed `-c` is
read again. A wrapper such as `sudo`, `env` or `xargs` is unwrapped. A writer handed no path at all
reads from a pipe, so the whole line is read for a path it names.

## What makes a file a test

The name, because the runner decides the same way. A name that ends in `_test.go`, `.spec.ts`,
`_spec.rb` or `.feature`. A name that begins with `test_`. A path under `test`, `tests`, `spec`,
`__tests__`, `features`, `fixtures` or `testdata`. A fixture is what a test asserts against. A change
to one changes the assertion, and it touches no file named as a test.

## What makes a directory a test

What is in it, read off the disk. A name says nothing about the contents. `rm -rf build/` is
ordinary work and `rm -rf internal/` takes every test in there with it. The two are the same sentence
to any rule about names.

The hook runs inside the sandbox, in the session's own working directory. So it walks the path and
answers from what it finds. The walk is bounded at twenty thousand entries, and it skips `.git`,
`node_modules`, `vendor` and `target`. A directory too big to read is refused rather than cleared.

A command that names no path is pointed at the working directory, so `git checkout -- .`,
`git stash` and `git clean -fd` are read as that directory.

## What it allows, on purpose

**Reading.** `cat`, `grep`, `sed -n '1,40p'` and running the suite are all allowed. This is the
deliberate difference from the discipline the stage comes from, where the implementer never sees the
test at all. A build that cannot read the test cannot tell a failing assertion from a broken one, so
it guesses instead.

## What it does not catch

This reads a command line. It does not run one, so three things go past it, and they are named here
rather than left for somebody to find.

A path that only exists after the shell expands it. A glob, a variable and a substitution are read as
the words they are written as, so `rm $TESTS` says nothing to this gate.

A program that writes through a name this reader does not know, in an argument that does not look like
a path. An unknown program is read for path shaped words. That catches the ordinary spelling and not a clever
one.

A directory the walk cannot see, because the session works somewhere else on the disk. Then the
name rules still answer, and they are what catch `features/` and `testdata/`.

Behind all three stands the stage itself. A build report names the files it wrote, and a report that
names a test is refused whatever the gate saw.

## The way through

The refusal names the file. It says why the file reads as a test. It says what to do instead. Read it.
If the test itself is wrong, say so in the answer, name the file and the assertion, and say what it
must assert. A person decides that.

The stage reads an answer of that shape and puts it to somebody. So a worker between a broken test and
this boundary is never stuck for long.
