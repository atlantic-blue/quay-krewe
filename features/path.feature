Feature: A project holds a numbered path of steps

  A design says what to build. It does not say what to build first, and a project had nowhere to keep
  the answer, so the atomised changes lived in whoever wrote them down.

  The path is that list, and it belongs to one feature of the project. A project delivers several
  features at once and each one has a path of its own, so writing one leaves the others whole and two
  features may each hold a step 3. Each step carries one intention, the files it writes, what proves
  it, and the name of the scenario krewe runs to check it.

  A step is named as one token, <feature>.<number>. A bare number was the whole address before the
  path belonged to a feature, so it is refused rather than guessed at.

  The control plane parses the document rather than the caller, so the console and the command line
  send the same words and cannot drift on the grammar.

  A step that names no predecessor waits for the number below it, and step one waits for nobody. A
  default of nothing would make the gate worthless, because every step would be ready at once.

  A feature is delivered in milestones. One heading with one hash names a milestone, and every step
  under it belongs to it, so a reader sees what a run of steps adds up to. Step numbers stay unique
  across the whole feature and do not start again under each milestone.

  A step that is taken, done or stopped is protected. A document that drops one of those numbers, or
  holds it under another title, is refused and writes nothing. The refusal names every protected step
  it found. A path is a record as well as a plan, and a rewrite that dropped a step somebody worked on
  would take that record away. A ready step is replaced whole, so a path can still be corrected.

  A step ends when somebody says what came of it. Done and stopped are the two words, and any other
  word is refused. The result is required, because nothing can see inside a container: what somebody
  wrote is what the next session reads. Finishing a step touches no session, so the step still says
  who took it, and a stopped step is not ready.

  Several steps run at once. The operator takes each one, so every session starts because somebody
  typed a command: no command reads the path and dispatches, and a step that finishes starts nothing.
  A cap on the project says how many steps may be in state taken at one time, and the default is ten.
  Ten is an observation: this project held ten steps in state taken at one moment on 9 September
  2026, and the default is that count, so the cap refuses no work the system was already doing. It is
  not a tuned number. The cap counts across every feature of the project rather than inside one, so
  three steps in flight is three steps wherever they sit, and the refusal names the feature each one
  is in.

  A step is also refused when it names a file that a step in flight names, so two sessions are never
  given one file. The check reads what each step says it writes, one file per line, across every
  feature of the project. It never reads a diff, so a step that writes a file it did not name is not
  caught. The refusal lands on the step and never on the feature: a feature waits on one step and
  carries on with the rest of its path, and finishing the step that holds the file lets the refused
  take through with nothing re-planned.

  A session that takes a step restates it before it builds anything. The take text tells it to write
  no code, and to write what it understood into its own memory file under one mark, in six named
  parts. The last part is a percentage, so the operator sees how sure the session is. The next exec
  reads that section into the step, and every exec after it renders the text back out of the store.
  The text is unapproved the moment it is written, and a second restatement is unapproved too: the
  operator agrees to one text and never to the step.

  The operator reads that text on demand. Reading a step reads the session's own file first, so what
  comes back is what the session understands now rather than what it understood at its last exec. It
  starts no container and asks no model, which is what lets the operator read a restatement in the
  moment it is written. A step that is done or stopped is answered from the store and its file is not
  read: the session may be gone, and the record of work that is over does not move. A file that
  cannot be read warns and refuses nothing, because the last text a session wrote is still the text
  to read.

  There is no way to empty a path. A document with no step heading is refused, so a wrong file path
  cannot take somebody's path away.

  The session working in the project reads the path in its own working directory, in number order.
  That is what lets a session start from what is true: a session on step 4 reads what steps 1 to 3
  produced. It reads the open features only, each under a heading of its own, and each step under the
  milestone it belongs to. A step block names its milestone as well, because a session opens the file
  at its own step and reads down.

  Background:
    Given a running control plane
    And a workspace named "acme"
    And a project named "house-bills"

  Scenario: A path of five steps is written and read back in number order
    When the operator sets the path to:
      """
      # The path

      Five steps, and the order they go in.

      ## 3. The control plane serves the design
      ## 1. The store holds a project's brief
      ## 5. The command line reads it back
      ## 2. The store holds a project's design
      ## 4. The session reads the design
      """
    And the operator reads the path
    Then the path holds 5 steps
    And the path reads 1, 2, 3, 4, 5 in that order
    And step 1 is titled "The store holds a project's brief"
    And step 5 is titled "The command line reads it back"

  Scenario: Every block of a step reaches the store
    When the operator sets the path to:
      """
      ## 1. The store holds a project's brief

      What changes and why
      The design has nowhere to live, so a project cannot carry one.

      What this touches
      internal/store/store.go
      internal/store/postgres.go

      What proves it
      The operator sets a brief and reads it back, so the project says what it is for.

      The scenario that proves it
      a project carries a brief

      After
      0
      """
    And the operator reads the path
    Then step 1 says its intention is "The design has nowhere to live, so a project cannot carry one."
    And step 1 touches "internal/store/store.go\ninternal/store/postgres.go"
    And step 1 says its proof is "The operator sets a brief and reads it back, so the project says what it is for."
    And step 1 names the scenario "a project carries a brief"
    And step 1 waits for step 0
    And step 1 is ready

  # A numbered path is a chain unless the document says otherwise. Every step ready at once is the
  # same as no order at all, which is what the gate exists to stop.
  Scenario: A step with no named predecessor waits for the number below it
    When the operator sets the path to:
      """
      ## 1. The store holds a project's brief
      ## 2. The store holds a project's design
      ## 3. The control plane serves the design
      """
    And the operator reads the path
    Then step 1 waits for step 0
    And step 2 waits for step 1
    And step 3 waits for step 2

  # Saying otherwise is an After block with nothing in it. It is the only way to say a step waits for
  # nobody, which is why an absent block cannot mean the same thing.
  Scenario: An After block holding nothing says the step waits for nobody
    When the operator sets the path to:
      """
      ## 1. The store holds a project's brief

      ## 2. The store holds a project's design

      After
      """
    And the operator reads the path
    Then step 2 waits for step 0

  Scenario: A step may wait for a step that is not the one below it
    When the operator sets the path to:
      """
      ## 1. The store holds a project's brief
      ## 2. The store holds a project's design
      ## 5. The command line reads it back

      After
      1
      """
    And the operator reads the path
    Then step 5 waits for step 1

  # The person fixing this has to see the line they forgot as well as the one they are looking at.
  Scenario: A document with a duplicate number is refused, naming both lines
    When the operator sets the path to:
      """
      ## 1. The store holds a project's brief
      ## 3. The control plane serves the design
      ## 3. The command line reads it back
      """
    Then the control plane refuses it as invalid
    And the refusal names lines 2 and 3
    And the project has no path

  # There is no way to empty a path, and that is deliberate: a wrong file path would otherwise delete
  # somebody's work with no refusal.
  Scenario: A document with no step heading is refused
    When the operator sets the path to:
      """
      # The path

      Five steps, and the order they go in. I never wrote the steps.
      """
    Then the control plane refuses it as invalid
    And the refusal suggests "## 1. <title>"

  Scenario: A path already written survives a document with no step heading
    Given the project's path is:
      """
      ## 1. The store holds a project's brief
      """
    When the operator sets the path to:
      """
      nothing in here is a step
      """
    Then the control plane refuses it as invalid
    And the operator reads the path
    And the path holds 1 steps

  Scenario: A step numbered zero is refused
    When the operator sets the path to:
      """
      ## 0. The store holds a project's brief
      """
    Then the control plane refuses it as invalid
    And the refusal names line 1

  Scenario: A step numbered below zero is refused
    When the operator sets the path to:
      """
      ## 1. The store holds a project's brief
      ## -2. The store holds a project's design
      """
    Then the control plane refuses it as invalid
    And the refusal names line 2

  # A number the column cannot hold is refused rather than wrapped. Wrapped, the document would say
  # one number and the store would hold another, with nothing saying so.
  Scenario: A step numbered larger than the column holds is refused
    When the operator sets the path to:
      """
      ## 4294967297. The store holds a project's brief
      """
    Then the control plane refuses it as invalid
    And the refusal names line 1
    And the refusal suggests "4294967297"

  Scenario: A step with no title is refused
    When the operator sets the path to:
      """
      ## 1. The store holds a project's brief
      ## 2.
      """
    Then the control plane refuses it as invalid
    And the refusal names line 2

  Scenario: An After that is not a number is refused
    When the operator sets the path to:
      """
      ## 1. The store holds a project's brief

      ## 2. The store holds a project's design

      After
      the first one
      """
    Then the control plane refuses it as invalid
    And the refusal names line 5

  Scenario: An After naming a step the document does not have is refused
    When the operator sets the path to:
      """
      ## 1. The store holds a project's brief

      ## 2. The store holds a project's design

      After
      7
      """
    Then the control plane refuses it as invalid
    And the refusal names line 5
    And the refusal suggests "no step 7"

  Scenario: An After that is not lower than the step's own number is refused
    When the operator sets the path to:
      """
      ## 1. The store holds a project's brief

      ## 2. The store holds a project's design

      After
      2
      """
    Then the control plane refuses it as invalid
    And the refusal names line 5

  # Krewe runs the scenario a step names, so a step that named two would leave it choosing.
  Scenario: A scenario block holding more than one line is refused
    When the operator sets the path to:
      """
      ## 1. The store holds a project's brief

      The scenario that proves it
      a project carries a brief
      a project carries a design
      """
    Then the control plane refuses it as invalid
    And the refusal names line 5

  # A step whose title needs the word "and" is two steps, and the design session enforces that. The
  # system never refuses a title for holding a word.
  Scenario: A title holding the word and is accepted
    When the operator sets the path to:
      """
      ## 1. Add the table and the index
      """
    And the operator reads the path
    Then step 1 is titled "Add the table and the index"

  # No warning refuses a document. The step is kept as it is and a person decides.
  Scenario: A step that says nothing under its labels warns and is kept
    When the operator sets the path to:
      """
      ## 1. The store holds a project's brief
      """
    Then the path write warns that step 1 says nothing under "What this touches"
    And the operator reads the path
    And the path holds 1 steps

  # Harder than the others, because this is the one krewe cannot work around: it runs the scenario a
  # step names, and there is nothing to run.
  Scenario: A step naming no scenario is warned about by name
    When the operator sets the path to:
      """
      ## 1. The store holds a project's brief
      """
    Then the path write warns "krewe step check will refuse this step"

  Scenario: A step that fills every block warns about nothing
    When the operator sets the path to:
      """
      ## 1. The store holds a project's brief

      What changes and why
      The design has nowhere to live.

      What this touches
      internal/store/store.go

      What proves it
      The operator sets a brief and reads it back.

      The scenario that proves it
      a project carries a brief

      The contracts it builds
      TABLE-1

      The scope of each contract
      TABLE-1: the columns migration 0062 creates. The approval column comes later.
      """
    Then the path write warns about nothing

  # The contracts a step builds, and which part of each one is this step's. Every step brief this
  # project wrote by hand carried that scoping, copied out of the graph by somebody reading it. The
  # document carries it now, so nobody types it and a session cannot be handed the wrong part of a
  # contract.

  Scenario: A step names the contracts it builds and the scope of each
    When the operator sets the path to:
      """
      ## 1. The store holds a project's brief

      The contracts it builds
      TABLE-1
      STORE-2

      The scope of each contract
      TABLE-1: the columns migration 0062 creates. The approval column comes later.
      STORE-2: the whole method.
      """
    And the operator reads the path
    Then step 1 builds the contracts "TABLE-1\nSTORE-2"
    And step 1 says the scope of each contract is "TABLE-1: the columns migration 0062 creates. The approval column comes later.\nSTORE-2: the whole method."

  # This is the defect the refusal exists for. A scope line about a contract the step does not build
  # reads as an answer about that contract, and the take text then hands the session the scope of
  # somebody else's work.
  Scenario: A scope line naming a contract the step does not build is refused, naming the line
    When the operator sets the path to:
      """
      ## 1. The store holds a project's brief

      The contracts it builds
      TABLE-1

      The scope of each contract
      TABLE-1: the columns migration 0062 creates.
      STORE-2: the whole method.
      """
    Then the control plane refuses it as invalid
    And the refusal names line 8
    And the refusal says "STORE-2"
    And the project has no path

  # Without the stop there is no telling the identifier from the sentence, so the refusal says the
  # form rather than the mistake.
  Scenario: A scope line that does not stop after the identifier is refused, and the refusal names the form
    When the operator sets the path to:
      """
      ## 1. The store holds a project's brief

      The contracts it builds
      TABLE-1

      The scope of each contract
      TABLE-1 the columns migration 0062 creates.
      """
    Then the control plane refuses it as invalid
    And the refusal names line 7
    And the refusal says "the form is <identifier>: <sentence>"

  # Both lines, because the person fixing it has to see the one they forgot as well as the one they
  # are looking at. One contract has one scope line, and two of them are two answers.
  Scenario: Two scope lines naming the same contract are refused, naming both lines
    When the operator sets the path to:
      """
      ## 1. The store holds a project's brief

      The contracts it builds
      TABLE-1

      The scope of each contract
      TABLE-1: the columns migration 0062 creates.
      TABLE-1: the approval column comes later.
      """
    Then the control plane refuses it as invalid
    And the refusal names lines 7 and 8

  # A warning and not a refusal. A contract identifier is a string the operator wrote, and krewe
  # never reads the contracts document, so it cannot tell a missing contract from a wrong one.
  Scenario: A step with an empty contracts block warns and is kept
    When the operator sets the path to:
      """
      ## 1. The store holds a project's brief

      The contracts it builds
      """
    Then the path write warns "step 1 names no contract"
    And the operator reads the path
    And the path holds 1 steps
    And step 1 builds the contracts ""

  Scenario: A contract with no scope line under it warns, and the warning names the contract
    When the operator sets the path to:
      """
      ## 1. The store holds a project's brief

      The contracts it builds
      TABLE-1
      STORE-2

      The scope of each contract
      TABLE-1: the columns migration 0062 creates.
      """
    Then the path write warns "step 1 builds STORE-2 and says nothing about its scope"
    And the operator reads the path
    And step 1 builds the contracts "TABLE-1\nSTORE-2"

  # A document written before these two labels existed is every document this project wrote until
  # now. It reads back with no contracts and no scope, and every other block whole.
  Scenario: A step naming no contract reads back with every other block whole
    When the operator sets the path to:
      """
      ## 1. The store holds a project's brief

      What changes and why
      The design has nowhere to live, so a project cannot carry one.

      What this touches
      internal/store/store.go

      What proves it
      The operator sets a brief and reads it back.

      The scenario that proves it
      a project carries a brief

      After
      0
      """
    And the operator reads the path
    Then step 1 builds the contracts ""
    And step 1 says the scope of each contract is ""
    And step 1 says its intention is "The design has nowhere to live, so a project cannot carry one."
    And step 1 touches "internal/store/store.go"
    And step 1 says its proof is "The operator sets a brief and reads it back."
    And step 1 names the scenario "a project carries a brief"
    And step 1 waits for step 0
    And step 1 is ready

  # The milestones. A feature is delivered in milestones, and one document carries the whole path of
  # one feature, so the milestone headings sit in the same document as the steps. One hash names a
  # milestone and two name a step, so a milestone heading sits above the steps under it.

  Scenario: A document with three milestone headings writes three milestone rows
    When the operator sets the path to:
      """
      # 1. A project carries a design
      The project has nowhere to keep what it is for.

      ## 1. The store holds a project's brief
      ## 2. The store holds a project's design

      # 2. A design carries an approval
      Nothing is built from a design nobody read.

      ## 3. The operator approves the design

      # 3. A design carries a numbered path
      The design has nowhere to keep the atomised changes.

      ## 4. The store holds a path
      """
    And the operator reads the path
    Then the path holds 3 milestones
    And the milestones read 1, 2, 3 in that order
    And milestone 1 is titled "A project carries a design"
    And milestone 3 is titled "A design carries a numbered path"
    And milestone 1 says its intention is "The project has nowhere to keep what it is for."
    And step 1 is in milestone 1
    And step 2 is in milestone 1
    And step 3 is in milestone 2
    And step 4 is in milestone 3

  # A path is a path before anybody groups it, so a document that names no milestone is not an error.
  Scenario: A document with no milestone heading gives every step a milestone of 0
    When the operator sets the path to:
      """
      ## 1. The store holds a project's brief
      ## 2. The store holds a project's design
      """
    And the operator reads the path
    Then the path holds 0 milestones
    And step 1 is in milestone 0
    And step 2 is in milestone 0

  Scenario: A step before the first milestone heading is in no milestone
    When the operator sets the path to:
      """
      ## 1. The store holds a project's brief

      # 1. A design carries a numbered path

      ## 2. The store holds a path
      """
    And the operator reads the path
    Then step 1 is in milestone 0
    And step 2 is in milestone 1

  # The trap in this grammar. A reader expects the numbering to start again under each milestone, and
  # it does not: a feature holds one step 1. So the refusal says that, rather than naming the number
  # and leaving the person to work out why two milestones did not make two paths.
  Scenario: Two steps numbered 1 under different milestones are refused, naming both lines
    When the operator sets the path to:
      """
      # 1. A project carries a design

      ## 1. The store holds a project's brief

      # 2. A design carries a numbered path

      ## 1. The store holds a path
      """
    Then the control plane refuses it as invalid
    And the refusal names lines 3 and 7
    And the refusal says "Step numbers are unique across the whole feature, and not inside a milestone"
    And the project has no path

  # A document may still open with a title, so a hash line carrying no number is ignored the way a
  # paragraph is rather than read as a milestone with no number.
  Scenario: A document opening with a hash line that carries no number parses, and that line is ignored
    When the operator sets the path to:
      """
      # The path

      Two steps, and the order they go in.

      ## 1. The store holds a project's brief
      ## 2. The store holds a project's design
      """
    And the operator reads the path
    Then the path holds 2 steps
    And the path holds 0 milestones
    And step 1 is in milestone 0

  Scenario: A milestone numbered zero is refused
    When the operator sets the path to:
      """
      # 0. A project carries a design

      ## 1. The store holds a project's brief
      """
    Then the control plane refuses it as invalid
    And the refusal names line 1
    And the project has no path

  Scenario: A document with a duplicate milestone number is refused, naming both lines
    When the operator sets the path to:
      """
      # 1. A project carries a design

      ## 1. The store holds a project's brief

      # 1. A design carries a numbered path

      ## 2. The store holds a path
      """
    Then the control plane refuses it as invalid
    And the refusal names lines 1 and 5
    And the project has no path

  Scenario: A milestone with no title is refused
    When the operator sets the path to:
      """
      # 1.

      ## 1. The store holds a project's brief
      """
    Then the control plane refuses it as invalid
    And the refusal names line 1
    And the project has no path

  # A milestone nobody planned steps for is worth seeing, so it is kept and said out loud.
  Scenario: A milestone with no step under it warns and is kept
    When the operator sets the path to:
      """
      # 1. A project carries a design

      # 2. A design carries a numbered path

      ## 1. The store holds a path
      """
    Then the path write warns "milestone 1 has no step under it"
    And the operator reads the path
    And the path holds 2 milestones

  Scenario: A milestone with no line under its heading warns and is kept
    When the operator sets the path to:
      """
      # 1. A project carries a design

      ## 1. The store holds a project's brief
      """
    Then the path write warns "milestone 1 says nothing under its heading"
    And the operator reads the path
    And the path holds 1 milestones

  # The milestones are replaced whole by the document that writes the steps, so a milestone the new
  # document does not carry is gone. A milestone holds no state of its own, so nothing is lost.
  Scenario: A second document replaces the milestones of the first
    When the operator sets the path to:
      """
      # 1. A project carries a design

      ## 1. The store holds a project's brief

      # 2. A design carries an approval

      ## 2. The operator approves the design
      """
    And the operator sets the path to:
      """
      # 5. A design carries a numbered path

      ## 1. The store holds a path
      """
    And the operator reads the path
    Then the path holds 1 milestones
    And the milestones read 5 in that order
    And step 1 is in milestone 5

  # A milestone number restarts in each feature, so a listing that names no feature carries no
  # milestones. Merged, milestone 1 of one feature and milestone 1 of another read as one.
  Scenario: Reading every feature's path answers with the steps and no milestones
    Given the project's feature "authentication"
    And the project's feature "payment"
    And the operator sets the path of feature 1 to:
      """
      # 1. Sign up works

      ## 1. Sign up
      """
    And the operator sets the path of feature 2 to:
      """
      # 1. Money moves

      ## 1. Checkout
      """
    When the operator reads the path of every feature
    Then the path holds 2 steps
    And the path holds 0 milestones

  Scenario: A feature with no path answers with nothing
    When the operator reads the path
    Then the path holds 0 steps

  # What is next is the operator's next command, put in front of them. It is a sentence and never a
  # dispatch: the read starts no session, takes no step and changes no row.
  Scenario: A path with the first two steps done says the third is next
    Given the project's path is:
      """
      ## 1. The store holds a project's brief
      ## 2. The store holds a project's design
      ## 3. The command line reads it back
      """
    And step 1 is recorded as done
    And step 2 is recorded as done
    When the operator reads the path
    Then the path says step 3 is next

  # A path nobody has touched. Step 1 waits for nobody, and it is the lowest step that is ready.
  Scenario: A path nobody has started says the first step is next
    Given the project's path is:
      """
      ## 1. The store holds a project's brief
      ## 2. The store holds a project's design
      """
    When the operator reads the path
    Then the path says step 1 is next

  # Both halves of the rule are read. Step 2 is the lowest ready step and it waits for a step that
  # stopped, so it is not next. Reading the state and skipping the predecessor names step 2 here, and
  # step 2 is the step nobody may take.
  Scenario: A ready step waiting for an unfinished step is not next
    Given the project's path is:
      """
      ## 1. The store holds a project's brief

      ## 2. The store holds a project's design

      ## 3. The command line reads it back

      After
      """
    And step 1 is recorded as stopped
    When the operator reads the path
    Then step 2 waits for step 1
    And the path says step 3 is next

  # A step somebody holds is not on offer, whatever it waits for.
  Scenario: A step somebody took is never next
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief

      ## 2. The store holds a project's design

      ## 3. The command line reads it back

      After
      """
    And the operator took step 1
    When the operator reads the path
    Then the path says step 3 is next

  Scenario: A step that is done or stopped is never next
    Given the project's path is:
      """
      ## 1. The store holds a project's brief
      ## 2. The store holds a project's design
      """
    And step 1 is recorded as done
    And step 2 is recorded as stopped
    When the operator reads the path
    Then the path says nothing is next

  # Every ready step waits for a step nobody finished, so there is nothing to start. Nothing is an
  # answer here, and it is not an error.
  Scenario: With every ready step waiting the path says nothing is next
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      ## 2. The store holds a project's design
      """
    And the operator took step 1
    When the operator reads the path
    Then the path says nothing is next

  Scenario: An empty path says nothing is next
    When the operator reads the path
    Then the path holds 0 steps
    And the path says nothing is next

  # What is next is a question about one path. Read across every feature, the lowest number would name
  # a step of a path nobody asked about.
  Scenario: A read naming no feature says nothing is next
    Given the project's feature "authentication"
    And the project's feature "payment"
    And the operator sets the path of feature 1 to:
      """
      ## 1. Sign up
      """
    And the operator sets the path of feature 2 to:
      """
      ## 1. Checkout
      """
    When the operator reads the path of every feature
    Then the path holds 2 steps
    And the path says nothing is next

  # The guard the whole slice stands on. Reading what is next is a sentence, so nothing starts and
  # nothing moves.
  Scenario: Reading what is next starts no session and moves no step
    Given the project's path is:
      """
      ## 1. The store holds a project's brief
      ## 2. The store holds a project's design
      """
    When the operator reads the path
    Then the path says step 1 is next
    And 0 sessions were started
    And step 1 is still ready
    And step 2 is still ready

  Scenario: The path of a feature that does not exist is refused
    When the operator reads the path of a feature that does not exist
    Then the control plane refuses it as not found

  Scenario: A path written for no feature at all is refused
    When the operator sets a path without saying which feature
    Then the control plane refuses it as invalid

  # The whole reason the key moved down to the feature. Keyed by the project, the second write wiped
  # the first, so a project could only ever be building one thing.
  Scenario: Setting the path of feature 2 leaves the path of feature 1 whole
    Given the project's feature "authentication"
    And the project's feature "payment"
    When the operator sets the path of feature 1 to:
      """
      ## 1. Sign up
      ## 2. Sign in
      """
    And the operator sets the path of feature 2 to:
      """
      ## 1. Checkout
      """
    And the operator reads the path of feature 1
    Then the path holds 2 steps
    And the path reads 1, 2 in that order
    And step 1 is titled "Sign up"

  # A path is a plan and it is a record. Somebody took step 2, so a document without it puts that
  # number back in the pile and leaves nothing saying the work happened.
  Scenario: A path that drops a step somebody took is refused, naming the step
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      ## 2. The store holds a project's design
      ## 3. The command line reads it back
      """
    And the operator took step 2
    When the operator sets the path to:
      """
      ## 1. The store holds a project's brief
      ## 3. The command line reads it back
      """
    Then the control plane refuses it as the wrong state
    And the refusal suggests "step 2 is taken"
    And the refusal suggests "keep those numbers in the document"

  # Three states are protected, not one. A step that is done holds the record of work that finished,
  # and a document that drops it takes that record away.
  #
  # Nothing finishes a step and nothing stops one yet, so these two scenarios write the state onto
  # the record directly. The call that finishes a step writes the same word.
  Scenario: A path that drops a step somebody finished is refused, naming the step
    Given the project's path is:
      """
      ## 1. The store holds a project's brief
      ## 2. The store holds a project's design
      """
    And step 2 is recorded as done
    When the operator sets the path to:
      """
      ## 1. The store holds a project's brief
      """
    Then the control plane refuses it as the wrong state
    And the refusal suggests "step 2 is done"

  Scenario: A path that drops a step somebody stopped is refused, naming the step
    Given the project's path is:
      """
      ## 1. The store holds a project's brief
      ## 2. The store holds a project's design
      """
    And step 2 is recorded as stopped
    When the operator sets the path to:
      """
      ## 1. The store holds a project's brief
      """
    Then the control plane refuses it as the wrong state
    And the refusal suggests "step 2 is stopped"

  # The number surviving is not the record surviving. A step 2 that reads as something else is a step
  # nobody did, and the session that took it built the title that was there.
  Scenario: A path that renames a step somebody took is refused, naming the step
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      ## 2. The store holds a project's design
      """
    And the operator took step 2
    When the operator sets the path to:
      """
      ## 1. The store holds a project's brief
      ## 2. Something else entirely
      """
    Then the control plane refuses it as the wrong state
    And the refusal suggests "step 2 is taken"

  # A refusal naming one of two sends the operator back to write the document a second time.
  Scenario: The refusal names every protected step the document would lose
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      ## 2. The store holds a project's design
      ## 3. The command line reads it back
      """
    And the operator took step 3
    And step 1 is recorded as done
    When the operator sets the path to:
      """
      ## 2. The store holds a project's design

      After
      """
    Then the control plane refuses it as the wrong state
    And the refusal suggests "step 1 is done"
    And the refusal suggests "step 3 is taken"

  # The rule protects the record and never the path. A ready step is replaced whole, or a document
  # could not be corrected once anything under it moved.
  Scenario: A path that drops a ready step goes
    Given the project's path is:
      """
      ## 1. The store holds a project's brief
      ## 2. The store holds a project's design
      ## 3. The command line reads it back
      """
    When the operator sets the path to:
      """
      ## 1. The store holds a project's brief
      ## 3. The command line reads it back
      """
    And the operator reads the path
    Then the path holds 2 steps
    And the path reads 1, 3 in that order

  # The correction the rule has to leave possible. The step before this one stopped, so it waits for
  # something else now.
  Scenario: Changing After on a ready step moves it past a stopped step
    Given the project's path is:
      """
      ## 1. The store holds a project's brief
      ## 2. The store holds a project's design
      ## 3. The command line reads it back
      """
    And step 2 is recorded as stopped
    When the operator sets the path to:
      """
      ## 1. The store holds a project's brief
      ## 2. The store holds a project's design
      ## 3. The command line reads it back

      After
      1
      """
    And the operator reads the path
    Then step 3 waits for step 1
    And step 2 is still stopped

  # The whole write is one transaction, so a refusal is not a partial rewrite. Step 3 is in the
  # document and step 1 is not, and neither of them moves.
  Scenario: A refused write leaves the whole path exactly as it was
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      ## 2. The store holds a project's design
      ## 3. The command line reads it back
      """
    And the operator took step 2
    When the operator sets the path to:
      """
      ## 3. The command line reads it back, rewritten

      After
      """
    Then the control plane refuses it as the wrong state
    And the operator reads the path
    And the path holds 3 steps
    And the path reads 1, 2, 3 in that order
    And step 1 is titled "The store holds a project's brief"
    And step 3 is titled "The command line reads it back"
    And step 2 is still taken

  # A write names one feature, and a refused write names it too. The other feature of the project
  # never enters the transaction.
  Scenario: A refused write leaves the path of another feature of the same project whole
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's feature "authentication"
    And the project's feature "payment"
    And the operator sets the path of feature 1 to:
      """
      ## 1. Sign up
      ## 2. Sign in
      """
    And the operator sets the path of feature 2 to:
      """
      ## 1. Checkout
      """
    And the operator took step 2 of feature 1
    When the operator sets the path of feature 1 to:
      """
      ## 1. Sign up
      """
    Then the control plane refuses it as the wrong state
    And the operator reads the path of feature 2
    And the path holds 1 steps
    And step 1 is titled "Checkout"

  # Step 3 of one feature and step 3 of another are two steps. Keyed by the project they were one,
  # and a project could not run two features at once without them colliding.
  Scenario: Two features each hold a step 3, and taking one leaves the other ready
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's feature "authentication"
    And the project's feature "payment"
    And the operator sets the path of feature 1 to:
      """
      ## 3. Reset the password

      After
      """
    And the operator sets the path of feature 2 to:
      """
      ## 3. Refund a payment

      After
      """
    When the operator takes step 3 of feature 1
    And the operator reads the path of feature 2
    Then step 3 is ready
    And the operator reads the path of feature 1
    And step 3 is titled "Reset the password"

  Scenario: Taking a step of a feature that does not exist is refused
    When the operator takes a step of a feature that does not exist
    Then the control plane refuses it as not found

  # A design session writes the path. Writing one grants it nothing it could not reach by dispatching,
  # so it is not one of the calls the operator keeps.
  Scenario: A design session may write the path
    When the driver sets the path to:
      """
      ## 1. The store holds a project's brief
      """
    And the operator reads the path
    Then the path holds 1 steps

  # These scenarios run the command line tool as a caller runs it: its own process, its own standard
  # output, its own exit status.

  Scenario: The operator writes a path from a file and reads it back
    Given the system listens on an address the tool can dial
    And a path file saying:
      """
      ## 1. The store holds a project's brief
      ## 2. The store holds a project's design
      """
    When the caller writes the path from that file
    And the caller reads the path
    Then standard output carries "The store holds a project's brief"
    And standard output carries "The store holds a project's design"
    And the command succeeds

  Scenario: A path of five steps prints five lines in number order
    Given the system listens on an address the tool can dial
    And a path file saying:
      """
      ## 1. The store holds a project's brief
      ## 2. The store holds a project's design
      ## 3. The control plane serves the design
      ## 4. The session reads the design
      ## 5. The command line reads it back
      """
    And the caller wrote the path from that file
    When the caller reads the path
    Then standard output lists 5 steps in number order
    And the command succeeds

  Scenario: A project with no feature tells the caller to add one
    Given the system listens on an address the tool can dial
    When the caller reads the path
    Then standard output carries "has no feature yet"
    And standard output carries "krewe feature add"
    And the command succeeds

  Scenario: A feature with no path tells the caller how to write one
    Given the system listens on an address the tool can dial
    And the project's feature "authentication"
    When the caller reads the path
    Then standard output carries "this feature has no path yet"
    And standard output carries "krewe path set"
    And the command succeeds

  # The listing groups the steps under the milestones the feature is delivered in, so the operator
  # reads where the feature reached without counting rows.
  #
  # The order is the control plane's, between milestones as well as inside one. Two surfaces draw this
  # path, and an order the tool worked out for itself lets the two disagree in front of somebody.

  Scenario: A path under two milestones prints a heading for each one
    Given the system listens on an address the tool can dial
    And a path file saying:
      """
      # 1. A project carries a design

      ## 1. The store holds a project's brief
      ## 2. The store holds a project's design
      ## 3. The control plane serves the design

      # 2. A design carries an approval

      ## 4. The operator approves the design
      ## 5. The command line reads it back
      """
    And the caller wrote the path from that file
    When the caller reads the path
    Then standard output carries "1. A project carries a design"
    And standard output carries "2. A design carries an approval"
    And standard output lists 5 step lines
    And standard output carries "5 steps, 5 ready."
    And the command succeeds

  Scenario: The milestones print in number order
    Given the system listens on an address the tool can dial
    And a path file saying:
      """
      # 1. A project carries a design

      ## 1. The store holds a project's brief

      # 2. A design carries an approval

      ## 2. The operator approves the design
      """
    And the caller wrote the path from that file
    When the caller reads the path
    Then the heading "1. A project carries a design" prints before the heading "2. A design carries an approval"
    And the command succeeds

  Scenario: Each milestone heading counts its own steps and how many are done
    Given the system listens on an address the tool can dial
    And a path file saying:
      """
      # 1. A project carries a design

      ## 1. The store holds a project's brief
      ## 2. The store holds a project's design
      ## 3. The control plane serves the design

      # 2. A design carries an approval

      ## 4. The operator approves the design
      ## 5. The command line reads it back
      """
    And the caller wrote the path from that file
    When the caller reads the path
    Then standard output carries "1. A project carries a design (3 steps, 0 done)"
    And standard output carries "2. A design carries an approval (2 steps, 0 done)"
    And the command succeeds

  # A step nobody gave a milestone still prints. A step that fell off the listing is the worst thing
  # this command can do, because what is left still reads as the whole path.
  Scenario: A step in no milestone prints under a heading of its own
    Given the system listens on an address the tool can dial
    And a path file saying:
      """
      ## 1. The store holds a project's brief

      # 1. A design carries an approval

      ## 2. The operator approves the design
      """
    And the caller wrote the path from that file
    When the caller reads the path
    Then standard output carries "no milestone (1 steps, 0 done)"
    And standard output lists 2 step lines
    And standard output carries "The store holds a project's brief"
    And the command succeeds

  Scenario: The steps in no milestone print before the numbered milestones
    Given the system listens on an address the tool can dial
    And a path file saying:
      """
      ## 1. The store holds a project's brief

      # 1. A design carries an approval

      ## 2. The operator approves the design
      """
    And the caller wrote the path from that file
    When the caller reads the path
    Then the heading "no milestone" prints before the heading "1. A design carries an approval"
    And the command succeeds

  Scenario: The line under the list counts the feature and names the next step
    Given the system listens on an address the tool can dial
    And a path file saying:
      """
      ## 1. The store holds a project's brief
      ## 2. The store holds a project's design
      """
    And the caller wrote the path from that file
    When the caller reads the path of feature 1
    Then standard output carries "2 steps, 2 ready."
    And standard output carries "next: step 1"
    And the command succeeds

  # Every step is taken or waits for one that is not done, so there is nothing to start. The line says
  # that rather than naming a step nobody may take.
  Scenario: With every step taken or waiting the next line names no step
    Given the system listens on an address the tool can dial
    And the project's design is "# Bills\n"
    And the operator approved the project's design
    And a path file saying:
      """
      ## 1. The store holds a project's brief
      ## 2. The store holds a project's design
      """
    And the caller wrote the path from that file
    And the operator takes step 1
    When the caller reads the path of feature 1
    Then standard output carries "2 steps, 1 taken, 1 ready."
    And standard output carries "next: nothing, every step is taken or waiting"
    And the command succeeds

  # The record of finished work stays readable: a closed feature is left out of the listing of the
  # project, and naming its number prints it.
  Scenario: A closed feature is left out when no feature number is given
    Given the system listens on an address the tool can dial
    And the project's feature "authentication"
    And the project's feature "payment"
    And the project's feature "the old import"
    And the operator sets the path of feature 1 to:
      """
      ## 1. Sign up
      """
    And the operator sets the path of feature 2 to:
      """
      ## 1. Checkout
      """
    And the operator sets the path of feature 3 to:
      """
      ## 1. Read the old file
      """
    And the operator closes feature 3
    When the caller reads the path
    Then standard output carries "feature 1: authentication"
    And standard output carries "feature 2: payment"
    And standard output does not carry "feature 3: the old import"
    And standard output carries "the project holds 2 steps, 2 ready."
    And the command succeeds

  Scenario: Naming a closed feature's number prints its path
    Given the system listens on an address the tool can dial
    And the project's feature "authentication"
    And the project's feature "the old import"
    And the operator sets the path of feature 2 to:
      """
      ## 1. Read the old file
      """
    And the operator closes feature 2
    When the caller reads the path of feature 2
    Then standard output carries "feature 2: the old import"
    And standard output carries "Read the old file"
    And the command succeeds

  Scenario: A feature number that names no feature is refused when the path is read
    Given the system listens on an address the tool can dial
    And the project's feature "authentication"
    When the caller reads the path of feature 9
    Then standard error says "has no feature 9"
    And standard error says "it has 1"
    And the command fails

  # A listing is a read. It says what is there and it moves nothing, so the operator reads it as often
  # as they want to.
  Scenario: Reading the path records nothing
    Given the system listens on an address the tool can dial
    And a path file saying:
      """
      ## 1. The store holds a project's brief
      ## 2. The store holds a project's design
      """
    And the caller wrote the path from that file
    When the caller reads the path
    And the caller reads the path
    Then every step of the path is ready and held by nobody
    And the command succeeds

  # A path belongs to a feature, so reading the project prints every open feature's path and the
  # heading above each one says which path is on the screen.
  Scenario: Two features print their paths under their own headings
    Given the system listens on an address the tool can dial
    And the project's feature "authentication"
    And the project's feature "payment"
    And the operator sets the path of feature 1 to:
      """
      ## 1. Sign up
      """
    And the operator sets the path of feature 2 to:
      """
      ## 1. Checkout
      """
    When the caller reads the path
    Then standard output carries "feature 1: authentication"
    And standard output carries "Sign up"
    And standard output carries "feature 2: payment"
    And standard output carries "Checkout"
    And the command succeeds

  Scenario: Reading one feature prints that feature's path and no other
    Given the system listens on an address the tool can dial
    And the project's feature "authentication"
    And the project's feature "payment"
    And the operator sets the path of feature 1 to:
      """
      ## 1. Sign up
      """
    And the operator sets the path of feature 2 to:
      """
      ## 1. Checkout
      """
    When the caller reads the path of feature 2
    Then standard output carries "Checkout"
    And standard output does not carry "Sign up"
    And the command succeeds

  Scenario: Writing one feature's path leaves another feature's path whole
    Given the system listens on an address the tool can dial
    And the project's feature "authentication"
    And the project's feature "payment"
    And the operator sets the path of feature 1 to:
      """
      ## 1. Sign up
      """
    And a path file saying:
      """
      ## 1. Checkout
      """
    When the caller writes the path of feature 2 from that file
    Then standard output carries "feature 2 of house-bills has a path of 1 steps"
    And the caller reads the path of feature 1
    And standard output carries "Sign up"
    And the command succeeds

  Scenario: A feature number that names no feature is refused, naming the numbers that exist
    Given the system listens on an address the tool can dial
    And the project's feature "authentication"
    And a path file saying:
      """
      ## 1. Checkout
      """
    When the caller writes the path of feature 9 from that file
    Then standard error says "has no feature 9"
    And standard error says "it has 1"
    And the command fails

  Scenario: A file with a duplicate step number is refused and nothing is written
    Given the system listens on an address the tool can dial
    And a path file saying:
      """
      ## 1. The store holds a project's brief
      ## 3. The control plane serves the design
      ## 3. The command line reads it back
      """
    When the caller writes the path from that file
    Then standard error says "line 2"
    And standard error says "line 3"
    And standard error says "nothing was written"
    And the command fails

  Scenario: Writing a path without saying which file is refused
    Given the system listens on an address the tool can dial
    When the caller writes the path without naming a file
    Then standard error says "usage: krewe path set"
    And the command fails

  Scenario: Writing a path without saying which feature is refused
    Given the system listens on an address the tool can dial
    And a path file saying:
      """
      ## 1. Checkout
      """
    When the caller writes the path without saying which feature
    Then standard error says "usage: krewe path set"
    And the command fails

  # The session working in the project reads the path out of its own working directory. It is a file
  # rather than a section in the memory file, because a path grows with the project and the memory
  # file is read on every exec.

  Scenario: A session reads every step of the path in its working directory
    Given the project's path is:
      """
      ## 1. The store holds a project's brief

      What changes and why
      The design has nowhere to live, so a project cannot carry one.

      What this touches
      internal/store/store.go
      internal/store/postgres.go

      What proves it
      The operator sets a brief and reads it back.

      The scenario that proves it
      a project carries a brief

      ## 2. The store holds a project's design
      ## 3. The control plane serves the design
      """
    When the operator dispatches "hello" to the project
    Then the session's path file lists steps 1, 2, 3 in that order
    And the session's path file carries "### 1. The store holds a project's brief"
    And the session's path file carries "state: ready"
    And the session's path file carries "The design has nowhere to live, so a project cannot carry one."
    And the session's path file carries "internal/store/postgres.go"
    And the session's path file carries "a project carries a brief"

  # Numbers need not run without gaps, so the order is the number's and never the store's. A file
  # that named 12 before 5 would be a different path from the one the operator wrote.
  Scenario: The steps are in number order even when the numbers are not contiguous
    Given the project's path is:
      """
      ## 5. The command line reads it back
      ## 1. The store holds a project's brief
      ## 12. The console shows the path
      ## 2. The store holds a project's design
      """
    When the operator dispatches "hello" to the project
    Then the session's path file lists steps 1, 2, 5, 12 in that order

  # A feature is delivered in milestones, so the document a session reads is grouped the way the
  # listing is: a heading for the feature, a heading for each milestone, and the steps under it.
  #
  # The milestone line in each block says what the heading above it says. A session opens this file at
  # its own step and reads down, so a block that named no milestone would leave it scrolling.
  Scenario: Each step block names the milestone it belongs to
    Given the project's path is:
      """
      # 1. A project carries a design

      It gives a design somewhere to live.

      ## 1. The store holds a project's brief
      ## 2. The store holds a project's design

      # 2. A design carries an approval

      ## 3. The operator approves the design
      """
    When the operator dispatches "hello" to the project
    Then the session's path file carries "## 1. A project carries a design"
    And the session's path file carries "## 2. A design carries an approval"
    And the session's path file carries "### 1. The store holds a project's brief\nmilestone: 1. A project carries a design"
    And the session's path file carries "### 3. The operator approves the design\nmilestone: 2. A design carries an approval"

  # The order is the number's between milestones as well as inside one, whatever order the document
  # declared them in. Written the other way round, the file would say step 3 comes first.
  Scenario: The steps are in number order inside a milestone and between milestones
    Given the project's path is:
      """
      # 2. A design carries an approval

      ## 4. The command line reads it back
      ## 3. The operator approves the design

      # 1. A project carries a design

      ## 2. The store holds a project's design
      ## 1. The store holds a project's brief
      """
    When the operator dispatches "hello" to the project
    Then the session's path file lists steps 1, 2, 3, 4 in that order

  # A step that fell out of the file because nobody gave it a milestone is the worst thing this
  # document can do, because what is left still reads as the whole path.
  Scenario: A step in no milestone is written under a heading of its own
    Given the project's path is:
      """
      ## 1. The store holds a project's brief

      # 1. A design carries an approval

      ## 2. The operator approves the design
      """
    When the operator dispatches "hello" to the project
    Then the session's path file lists steps 1, 2 in that order
    And the session's path file carries "## No milestone"
    And the session's path file carries "### 1. The store holds a project's brief\nmilestone: no milestone"

  # A file that exists and says nothing costs a read.
  Scenario: A project with no path has no path file
    Given the project's design is "# Bills\n"
    When the operator dispatches "hello" to the project
    Then the session has no path file
    And the session's memory file does not carry ".krewe/path.md"

  # A write that fails leaves the session without the file. It never fails the exec: the session works
  # in the project whether or not the render reached its working directory.
  Scenario: A path that cannot be written does not fail the exec
    Given the project's design is "# Bills\n"
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      """
    And a session started by dispatching "hello"
    And the path document cannot be written
    When the operator dispatches "and again" to the same session
    Then the session was asked "and again"
    And the session's memory file does not carry ".krewe/path.md"

  Scenario: Setting a new path and dispatching again gives the session the new text
    Given the project's path is:
      """
      ## 1. The store holds a project's brief
      """
    And a session started by dispatching "hello"
    When the operator sets the path to:
      """
      ## 1. The store holds a project's design
      """
    And the operator dispatches "and again" to the same session
    Then the session's path file carries "### 1. The store holds a project's design"
    And the session's path file does not carry "The store holds a project's brief"

  # A line naming a file that is not there sends the model to open nothing, so the summary names the
  # path only when the project has one.
  Scenario: The memory file sends the session to the path file
    Given the project's design is "# Bills\n"
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      """
    When the operator dispatches "hello" to the project
    Then the session's memory file carries "Read .krewe/design.md before you start. The whole path is in .krewe/path.md."

  # The section is read again on every exec of every session in the project, and the path took a
  # piece of it. The brief is the only part whose length nobody controls, so it is the part that is
  # cut.
  Scenario: A very long brief is cut so the section stays small with a path in it
    Given the project's brief is 5000 characters
    And the project's design is "# Bills\n"
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      """
    When the operator dispatches "hello" to the project
    Then the design section is under 400 characters
    And the session's memory file carries "The whole path is in .krewe/path.md."

  # Taking a step starts a session on it, and the system composes what that session is given: the
  # step whole, where it sits in the path, and what to do with it. The command line sends a number
  # and the control plane sends the words, so the console cannot send different ones.

  Scenario: Taking a step dispatches a session whose text names the step and its title
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief

      What changes and why
      The design has nowhere to live, so a project cannot carry one.

      What this touches
      internal/store/store.go

      What proves it
      The operator sets a brief and reads it back.

      The scenario that proves it
      a project carries a brief

      ## 2. The store holds a project's design
      ## 3. The control plane serves the design
      """
    When the operator takes step 1
    Then the step text carries "Step 1 of 3 on the path for house-bills."
    And the step text carries "The store holds a project's brief"
    And the step text carries "The design has nowhere to live, so a project cannot carry one."
    And the step text carries "internal/store/store.go"
    And the step text carries "The scenario that proves it is named: a project carries a brief"
    And the session was asked exactly what the take composed
    And step 1 is held by that session

  # The count is of the steps in the path and never of the highest number, so a path running 1, 2, 5
  # reads "of 3".
  Scenario: The count is the steps in the path and not the numbers
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      ## 2. The store holds a project's design
      ## 5. The command line reads it back
      """
    When the operator takes step 5
    Then the step text carries "Step 5 of 3 on the path for house-bills."

  # The last paragraph is always there, whole. It is what makes the session restate the step rather
  # than build it, and the mark it names is the mark the read back looks for.
  Scenario: The text tells the session to write no code and to restate the step
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      """
    When the operator takes step 1
    Then the step text carries "The design is in .krewe/design.md. The whole path is in .krewe/path.md. Read both."
    And the step text carries "Write no code. Change no file in the repository."
    And the step text carries "<!-- quay:restatement -->"
    And the step text carries "what this step changes, what it will not touch, what you assumed, what you do not know,"
    And the step text carries "the scenario you will write and the value it describes, how sure you are and what lowers it."
    And the step text carries "Then stop and say you are ready."

  # A step built and nothing delivered is the state every step of feature 1 was left in, and a person
  # wrote the delivery words into each dispatch by hand. The text carries them now. It names the two
  # skills rather than restating them, because the session already holds both.
  Scenario: The text tells the session to deliver the step as one pull request
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      """
    When the operator takes step 1
    Then the step text carries "Deliver this step as one pull request. The git and github skills say how a working tree, a branch, a commit and a pull request are done here."
    And the step text carries "Watch what proves this step fail before you make it pass, because a test you did not see fail proves nothing."
    And the step text carries "Get every check green before you report, and fix a red check rather than explaining it."
    And the step text carries "Do not merge it. Report the address of the pull request."
    And the step text carries "Read both.\n\nDeliver this step as one pull request."

  # A label with nothing under it is text the model has to read for nothing.
  Scenario: A step with no proof produces text with no proof label in it
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief

      What changes and why
      The design has nowhere to live, so a project cannot carry one.
      """
    When the operator takes step 1
    Then the step text carries "What changes and why"
    And the step text does not carry "What proves it"
    And the step text does not carry "The scenario that proves it"
    And the step text does not carry "What this touches"

  # The scoping the operator wrote by hand until now. The session is given the contracts it builds,
  # what part of each one is this step's, and the file that says what those contracts are.
  Scenario: Taking a step dispatches a session whose text names both contracts and both scopes
    Given the project's design is "# Bills\n"
    And the project's contracts are "# The contracts\nTABLE-1: table project_designs\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief

      The contracts it builds
      TABLE-1
      STORE-2

      The scope of each contract
      TABLE-1: the columns migration 0062 creates. The approval column comes later.
      STORE-2: the whole method.
      """
    When the operator takes step 1
    Then the step text carries "The contracts this step builds"
    And the step text carries "TABLE-1\nSTORE-2"
    And the step text carries "The scope of each contract"
    And the step text carries "TABLE-1: the columns migration 0062 creates. The approval column comes later."
    And the step text carries "STORE-2: the whole method."
    And the step text carries "The contracts are in .krewe/contracts.md"
    And the session was asked exactly what the take composed

  # A label with nothing under it is text the model reads for nothing, and a pointer to a file that
  # is not there sends it to open nothing.
  Scenario: A step with no contracts produces take text with no contracts label in it
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief

      What changes and why
      The design has nowhere to live, so a project cannot carry one.
      """
    When the operator takes step 1
    Then the step text carries "What changes and why"
    And the step text does not carry "The contracts this step builds"
    And the step text does not carry "The scope of each contract"
    And the step text does not carry ".krewe/contracts.md"

  # What the session understood, before any code exists. It writes the six parts under the
  # restatement mark in its own memory file, and the next exec reads that section into the step. The
  # text travels through a file because a model writes files and cannot make a call.

  Scenario: A session that writes the mark into its memory file has that text read back into the step
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      """
    When the operator takes step 1
    And the session writes its restatement:
      """
      What this step changes
      The store holds a project's brief.

      How sure I am
      90 per cent.
      """
    And the operator dispatches "and again" to the same session
    Then step 1 reads back the restatement "The store holds a project's brief."
    And step 1 reads back the restatement "90 per cent."
    And nobody has approved step 1's restatement

  # Text under a mark this build does not know is swept into the innermost level and stored as though
  # the operator had typed it. Swept there, the restatement is context from then on, and the next
  # render writes it underneath itself.
  Scenario: Text under the mark is not swept into the session's own context
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      """
    When the operator takes step 1
    And the session writes its restatement:
      """
      What this step changes
      The store holds a project's brief.
      """
    And the operator dispatches "and again" to the same session
    Then step 1 reads back the restatement "The store holds a project's brief."
    And the session's context does not carry "The store holds a project's brief."

  # The whole inner file is written from the store on every exec, so a section it does not render is
  # a section that disappears, and the session would be asked to restate a step it had already
  # restated.
  Scenario: A second exec of the same session still carries the restatement in its memory file
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      """
    When the operator takes step 1
    And the session writes its restatement:
      """
      What this step changes
      The store holds a project's brief.
      """
    And the operator dispatches "and again" to the same session
    And the operator dispatches "once more" to the same session
    Then the session's memory file carries "The store holds a project's brief."

  # Approval is a statement about one text and never about the step, so a text nobody has read cannot
  # inherit the word spoken over the text before it.
  Scenario: A second, different restatement clears the approval
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      """
    When the operator takes step 1
    And the session writes its restatement:
      """
      What this step changes
      The first reading.
      """
    And the operator dispatches "and again" to the same session
    And the session writes its restatement:
      """
      What this step changes
      The second reading.
      """
    And the operator dispatches "once more" to the same session
    Then step 1 reads back the restatement "The second reading."
    And step 1 does not read back the restatement "The first reading."
    And nobody has approved step 1's restatement

  # Once the step is closed the work is over, and the text would otherwise be read again on every
  # exec of a session that has moved on to something else.
  Scenario: Marking the step done removes the section from the next exec's memory file
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      """
    When the operator takes step 1
    And the session writes its restatement:
      """
      What this step changes
      The store holds a project's brief.
      """
    And the operator dispatches "and again" to the same session
    And the operator finishes step 1 with "shipped as pull request 800"
    And the operator dispatches "once more" to the same session
    Then the session's memory file does not carry "The store holds a project's brief."
    And step 1 reads back the restatement "The store holds a project's brief."

  # The whole point of the read: the operator sees what the session understood in the moment it was
  # written. The text travels through a file, so a read that only looked at the store would answer
  # with what the session understood at its last exec, and the operator would have to dispatch one to
  # find out. Nothing here asks the model anything: the take is the only exec in this scenario.
  Scenario: Reading a step shows the text the session wrote, with no dispatch in between
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      """
    When the operator takes step 1
    And the session writes its restatement:
      """
      What this step changes
      The store holds a project's brief.
      """
    And the operator reads step 1
    Then the restatement read carries "The store holds a project's brief."
    And the read warns about nothing
    And the model was asked 1 thing in all

  # A session that has gone still leaves the last text it wrote behind it. Refusing the read would
  # take that away as well, so the file is a warning and never an error.
  Scenario: Reading a step whose session is gone answers from the store, with a warning
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      """
    When the operator takes step 1
    And the session writes its restatement:
      """
      What this step changes
      The store holds a project's brief.
      """
    And the operator reads step 1
    And the session's own directory is gone
    And the operator reads step 1
    Then the restatement read carries "The store holds a project's brief."
    And the read warns "the session's own file could not be read"

  # No length refuses text a person or a session wrote. Past the length that six parts about one step
  # take, the read says so and still hands back every character.
  Scenario: A restatement past the length mark comes back whole, with a warning
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      """
    When the operator takes step 1
    And the session writes a restatement of 2500 characters
    And the operator reads step 1
    Then the restatement read is 2500 characters
    And the read warns "2500 characters"

  # A finished step is a record, and a record does not move. The session may be gone, and a session
  # that carried on writing must not change what a step that is over was restated as.
  Scenario: A step that is done is answered from the store, and its file is not read
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      """
    When the operator takes step 1
    And the session writes its restatement:
      """
      What this step changes
      The first reading.
      """
    And the operator reads step 1
    And the operator finishes step 1 with "shipped as pull request 800"
    And the session writes its restatement:
      """
      What this step changes
      The second reading.
      """
    And the operator reads step 1
    Then the restatement read carries "The first reading."
    And the restatement read does not carry "The second reading."

  # One step is one session's. Two takes that both passed would put two sessions on one change.
  Scenario: Taking a step somebody already holds is refused, naming the session
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      """
    And the operator took step 1
    When the operator takes step 1
    Then the control plane refuses it as the wrong state
    And the refusal names the session holding step 1
    And 1 session was started

  # Gate 1. No code exists before the operator approves the path, so this refusal comes before every
  # other check, costs one line of output, and starts nothing.
  Scenario: Taking a step on a design nobody approved is refused and starts nothing
    Given the project's design is "# Bills\n"
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      """
    When the operator takes step 1
    Then the control plane refuses it as the wrong state
    And the refusal suggests "krewe design approve"
    And 0 sessions were started
    And the operator reads the path
    And step 1 is ready

  Scenario: A design approved and then rewritten refuses the take again
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      """
    And the operator writes the project's design as "# Bills, rewritten\n"
    When the operator takes step 1
    Then the control plane refuses it as the wrong state
    And 0 sessions were started

  Scenario: Taking a step the path does not hold is refused, saying how many it has
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      ## 2. The store holds a project's design
      """
    When the operator takes step 7
    Then the control plane refuses it as not found
    And the refusal suggests "it has 2 steps"
    And 0 sessions were started

  Scenario: Taking a step numbered below one is refused
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    When the operator takes step 0
    Then the control plane refuses it as invalid
    And the refusal suggests "counts from one"

  # Milestone 7. Several steps run at once, capped per project.
  #
  # The operator takes each step, so a second take is a second command somebody typed. No command
  # reads the path and dispatches, and no step starts because another one ended.
  Scenario: Taking a second step while the first runs leaves both taken, each with its own session
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 4. The store holds a project's brief

      After

      ## 6. The command line reads it back

      After
      """
    And the operator took step 6
    When the operator takes step 4
    Then step 4 is held by that session
    And the operator reads the path
    And step 4 is still taken
    And step 6 is still taken
    And step 4 and step 6 name different sessions
    And 2 sessions were started

  # The cap is lowered to 3 first, so the refusal is reached in three takes rather than in ten. What
  # is proved is the rule, and the rule reads the project's own number.
  Scenario: A fourth take is refused, naming the cap and the three steps in flight
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief

      After

      ## 2. The store holds a project's design

      After

      ## 3. The command line reads it back

      After

      ## 4. The session reads the design

      After
      """
    And the operator caps the steps in flight at 3
    And the operator took step 1
    And the operator took step 2
    And the operator took step 3
    When the operator takes step 4
    Then the control plane refuses it as too many steps at once
    And the refusal suggests "cap is 3"
    And the refusal names the steps in flight with the feature each one sits in
    And the refusal suggests "krewe path cap"
    And 3 sessions were started
    And the operator reads the path
    And step 4 is ready

  # The count joins the steps to the features on the project. Counted inside one feature, a project
  # with three features could run three times its cap while every number still read the same.
  Scenario: Three steps taken in three different features refuse a fourth take in any feature
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's feature "authentication"
    And the project's feature "payment"
    And the project's feature "delivery"
    And the operator sets the path of feature 1 to:
      """
      ## 1. Sign up

      After

      ## 2. Sign in

      After
      """
    And the operator sets the path of feature 2 to:
      """
      ## 1. Checkout
      """
    And the operator sets the path of feature 3 to:
      """
      ## 1. Post it
      """
    And the operator caps the steps in flight at 3
    And the operator took step 1 of feature 1
    And the operator took step 1 of feature 2
    And the operator took step 1 of feature 3
    When the operator takes step 2 of feature 1
    Then the control plane refuses it as too many steps at once
    And the refusal suggests "step 1.1 authentication"
    And the refusal suggests "step 2.1 payment"
    And the refusal suggests "step 3.1 delivery"
    And 3 sessions were started

  Scenario: Raising the cap lets the fourth take through
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief

      After

      ## 2. The store holds a project's design

      After

      ## 3. The command line reads it back

      After

      ## 4. The session reads the design

      After
      """
    And the operator caps the steps in flight at 3
    And the operator took step 1
    And the operator took step 2
    And the operator took step 3
    And the operator caps the steps in flight at 4
    When the operator takes step 4
    Then step 4 is held by that session
    And the take says 4 of 4 steps are in flight
    And 4 sessions were started

  Scenario: Finishing one of three steps lets the next take pass
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief

      After

      ## 2. The store holds a project's design

      After

      ## 3. The command line reads it back

      After

      ## 4. The session reads the design

      After
      """
    And the operator caps the steps in flight at 3
    And the operator took step 1
    And the operator took step 2
    And the operator took step 3
    And the operator finishes step 1 with "the brief is kept"
    When the operator takes step 4
    Then step 4 is held by that session
    And the take says 3 of 3 steps are in flight

  # The guard on the fan out. A step that finishes starts nothing: krewe moves one row and writes one
  # line, and the next take waits for the operator to type it.
  Scenario: Finishing a step starts no session and takes no step
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief

      After

      ## 2. The store holds a project's design

      After
      """
    And the operator took step 1
    When the operator finishes step 1 with "the brief is kept"
    Then 1 session was started
    And step 1 is still done
    And step 2 is still ready
    And no step but step 1 names a session

  Scenario: The take says how many steps are in flight and what the cap is
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief

      After

      ## 2. The store holds a project's design

      After
      """
    And the operator took step 1
    When the operator takes step 2
    Then the take says 2 of 10 steps are in flight

  # Ten is what this project had in state taken at one moment on 9 September 2026. It is the number
  # the system was already running, taken as the default so the cap refuses no work it already does.
  Scenario: A project nobody configured caps the steps in flight at ten
    Then the cap on steps in flight is 10

  # Lowering it refuses the next take. It never stops a session that already runs.
  Scenario: Lowering the cap below what runs now leaves both sessions running
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief

      After

      ## 2. The store holds a project's design

      After

      ## 3. The command line reads it back

      After
      """
    And the operator took step 1
    And the operator took step 2
    When the operator caps the steps in flight at 1
    Then step 1 is still taken
    And step 2 is still taken
    And 2 sessions were started
    And the operator takes step 3
    And the control plane refuses it as too many steps at once

  Scenario: A cap of zero is refused, saying why zero is wrong
    When the operator caps the steps in flight at 0
    Then the control plane refuses it as invalid
    And the refusal suggests "refuse every take"
    And the cap on steps in flight is 10

  Scenario: A cap above twenty is refused
    When the operator caps the steps in flight at 21
    Then the control plane refuses it as invalid
    And the cap on steps in flight is 10

  # The cap is how much the operator reads at once. A session that could raise its own would widen
  # the fan out without anybody asking for it.
  Scenario: A session cannot set the cap
    When the driver asks to cap the steps in flight
    Then the driver is refused, told the call is the operator's to make
    And the cap on steps in flight is 10

  Scenario: The tool says how many steps are in flight after a take
    Given the system listens on an address the tool can dial
    And the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief

      After

      ## 2. The store holds a project's design

      After
      """
    And the operator took step 1
    When the caller takes step "1.2"
    Then standard output says "2 of 10 steps in flight"
    And the command succeeds

  Scenario: The tool prints the cap and what runs now, and writes nothing
    Given the system listens on an address the tool can dial
    And the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief

      After

      ## 2. The store holds a project's design

      After
      """
    And the operator took step 1
    When the caller reads the cap on steps in flight
    Then standard output says "10 steps at once, 1 in flight"
    And standard output says "9 September 2026"
    And the cap on steps in flight is 10
    And the command succeeds

  Scenario: The tool sets the cap
    Given the system listens on an address the tool can dial
    When the caller caps the steps in flight at "5"
    Then standard output says "5 steps at once"
    And the cap on steps in flight is 5
    And the command succeeds

  Scenario: The tool prints one line of refusal for a cap of zero
    Given the system listens on an address the tool can dial
    When the caller caps the steps in flight at "0"
    Then the command fails
    And the cap on steps in flight is 10

  # Milestone 7, the second limit. Two sessions writing one file write over each other, and the only
  # thing the system can see it coming with is what each step says it writes.
  #
  # The check reads the whole project, across every feature, for the reason the cap does. Two features
  # share the user model, the router and the configuration, so the collision is real and it crosses
  # the two paths.
  #
  # It reads what a step says it writes and never a diff, so a step that writes a file it did not name
  # is not caught. Section 14 of the design defers that check.
  Scenario: A take on a file a step in flight writes is refused, naming the file and that step
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief

      What this touches
      internal/store/store.go

      After

      ## 2. The command line reads it back

      What this touches
      internal/store/store.go

      After
      """
    And the operator took step 1
    When the operator takes step 2
    Then the control plane refuses it as a file two steps write
    And the refusal suggests "internal/store/store.go"
    And the refusal suggests "step 1.1 the bills"
    And 1 session was started
    And the operator reads the path
    And step 2 is ready

  # The refusal lands on the step and never on the feature. Nothing about payment is blocked, queued
  # or ordered against authentication: one step of it waits, and the rest of its path is there to
  # take.
  Scenario: A step of one feature sharing a file with a taken step of another is refused
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's feature "authentication"
    And the project's feature "payment"
    And the operator sets the path of feature 1 to:
      """
      ## 1. Sign up

      What this touches
      internal/user/model.go
      """
    And the operator sets the path of feature 2 to:
      """
      ## 1. Checkout

      What this touches
      internal/user/model.go

      After

      ## 2. The receipt

      What this touches
      internal/receipt/receipt.go

      After
      """
    And the operator took step 1 of feature 1
    When the operator takes step 1 of feature 2
    Then the control plane refuses it as a file two steps write
    And the refusal suggests "authentication"
    And the refusal suggests "internal/user/model.go"

  Scenario: The next step of the second feature is taken while the first one waits
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's feature "authentication"
    And the project's feature "payment"
    And the operator sets the path of feature 1 to:
      """
      ## 1. Sign up

      What this touches
      internal/user/model.go
      """
    And the operator sets the path of feature 2 to:
      """
      ## 1. Checkout

      What this touches
      internal/user/model.go

      After

      ## 2. The receipt

      What this touches
      internal/receipt/receipt.go

      After
      """
    And the operator took step 1 of feature 1
    And the operator takes step 1 of feature 2
    And the control plane refuses it as a file two steps write
    When the operator takes step 2 of feature 2
    Then step 2 of feature 2 is held by that session
    And the operator reads the path of feature 2
    And step 1 is ready
    And the path holds 2 steps

  Scenario: Two steps naming different files are both taken
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief

      What this touches
      internal/store/store.go

      After

      ## 2. The command line reads it back

      What this touches
      cmd/krewe/step.go

      After
      """
    And the operator took step 1
    When the operator takes step 2
    Then step 2 is held by that session
    And step 1 is still taken
    And 2 sessions were started

  # The document is prose somebody typed, so a line is trimmed at both ends before anything is
  # compared. A file written with a space after it is the same file.
  #
  # The spaced line sits above another one, because a block is trimmed whole before it is stored and a
  # single line would lose its spaces on the way in. What is proved here is the trim the take does.
  Scenario: A file written with trailing spaces still collides
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief

      What this touches
      internal/store/store.go   
      internal/store/memory.go

      After

      ## 2. The command line reads it back

      What this touches
      internal/store/store.go

      After
      """
    And the operator took step 1
    When the operator takes step 2
    Then the control plane refuses it as a file two steps write
    And the refusal suggests "internal/store/store.go"

  # The comparison is exact and case sensitive, because it compares text and never resolves a path.
  # This is the cost of that, written down so the limit is a decision somebody reads.
  Scenario: A file written two ways is two files to the check
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief

      What this touches
      internal/store/store.go

      After

      ## 2. The command line reads it back

      What this touches
      ./internal/store/store.go

      After

      ## 3. The console draws it

      What this touches
      internal/store/Store.go

      After
      """
    And the operator took step 1
    When the operator takes step 2
    Then step 2 is held by that session
    And the operator takes step 3
    And step 3 is held by that session
    And 3 sessions were started

  # A step that names no file matches nothing, so it is taken whatever it goes on to write. The path
  # write is where that is said out loud, because the document is where somebody can fix it.
  Scenario: A step naming no file is taken, and setting that path warned
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief

      What this touches
      internal/store/store.go

      After

      ## 2. The thinking

      After
      """
    And the path write warns "step 2 names no file"
    And the path write warns "collides with nothing"
    And the operator took step 1
    When the operator takes step 2
    Then step 2 is held by that session
    And 2 sessions were started

  # What the refusal tells the operator to do, so it has to work. Nothing is re-planned: the same
  # take, typed again, goes through.
  Scenario: Finishing the step that holds the file lets the refused take pass
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief

      What this touches
      internal/store/store.go

      After

      ## 2. The command line reads it back

      What this touches
      internal/store/store.go

      After
      """
    And the operator took step 1
    And the operator takes step 2
    And the control plane refuses it as a file two steps write
    And the operator finishes step 1 with "the brief is kept"
    When the operator takes step 2
    Then step 2 is held by that session
    And the take says 1 of 10 steps are in flight

  Scenario: The tool prints the shared file refusal and starts nothing
    Given the system listens on an address the tool can dial
    And the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief

      What this touches
      internal/store/store.go

      After

      ## 2. The command line reads it back

      What this touches
      internal/store/store.go

      After
      """
    And the operator took step 1
    When the caller takes step "1.2"
    Then the command fails
    And standard error says "internal/store/store.go"
    And standard error says "nothing was started"
    And 1 session was started

  # The session reads which step it is on in the design section of its own memory file, which it
  # reads on every exec.
  Scenario: The session that took a step reads which step it is on
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      ## 2. The store holds a project's design
      ## 3. The control plane serves the design
      """
    When the operator takes step 2
    Then the session's memory file carries "You are on step 2 of 3: The store holds a project's design"
    And the design section is under 400 characters

  # A session works in a project and its step may sit in any feature of it, so the search for the
  # step it holds is across the whole project. Narrowed to one feature, a session on a step of the
  # second one reads no step line at all.
  #
  # The count beside it is the other half. It is the step's own feature's path, because a path
  # belongs to a feature and "step 1 of 2" is a sentence about one path. The two features here hold
  # different numbers of steps, so a count taken across the project says 5.
  Scenario: A session on a step of the second feature reads which step it is on
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's feature "authentication"
    And the project's feature "payment"
    And the operator sets the path of feature 1 to:
      """
      ## 1. Sign up
      ## 2. Sign in
      ## 3. Reset the password
      """
    And the operator sets the path of feature 2 to:
      """
      ## 1. Checkout
      ## 2. Refund a payment
      """
    When the operator takes step 1 of feature 2
    Then the session's memory file carries "You are on step 1 of 2: Checkout"

  # The session reads the whole project in its working directory, because it works in the project
  # rather than in one feature of it. Each feature is under a heading of its own: two features each
  # hold a step 1, and without the headings the file reads as one path that counts up twice.
  Scenario: The session's path file carries every open feature's path under its own heading
    Given the project's feature "authentication"
    And the project's feature "payment"
    And the operator sets the path of feature 1 to:
      """
      ## 1. Sign up
      """
    And the operator sets the path of feature 2 to:
      """
      ## 1. Checkout
      """
    When the operator dispatches "hello" to the project
    Then the session's path file carries "# 1. authentication"
    And the session's path file carries "# 2. payment"
    And the session's path file carries "### 1. Sign up"
    And the session's path file carries "### 1. Checkout"

  # A session nobody gave a step to is most sessions, and the line costs context on every one of them.
  Scenario: A session that took no step reads no step line
    Given the project's design is "# Bills\n"
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      """
    When the operator dispatches "hello" to the project
    Then the session's memory file does not carry "You are on step"

  # These scenarios run the command line tool as a caller runs it: its own process, its own standard
  # output, its own exit status.

  Scenario: The operator takes a step and reads what the session was asked to do
    Given the system listens on an address the tool can dial
    And the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      """
    When the caller takes step "1.1"
    Then standard output carries "step 1.1 of house-bills is taken: The store holds a project's brief"
    And standard output carries "Step 1 of 1 on the path for house-bills."
    And standard output carries "Write no code. Change no file in the repository."
    And standard output carries "it will restate the step and build nothing"
    And the command succeeds

  # What the operator does between taking a step and agreeing to it. The six parts print whole, so
  # the output can be piped, and the approval prints beside them because agreement is about one text.
  Scenario: The operator reads the six parts the session wrote
    Given the system listens on an address the tool can dial
    And the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      """
    When the operator takes step 1
    And the session writes its restatement:
      """
      What this step changes
      The store holds a project's brief.

      What it will not touch
      The console.

      What I assumed
      The migration runs before the read.

      What I do not know
      Whether the brief has a length cap.

      The scenario I will write
      A project carries a brief and reads it back.

      How sure I am
      90 per cent.
      """
    And the caller reads the restatement of step "1.1"
    Then standard output carries "What this step changes"
    And standard output carries "What it will not touch"
    And standard output carries "What I assumed"
    And standard output carries "What I do not know"
    And standard output carries "The scenario I will write"
    And standard output carries "How sure I am"
    And standard output carries "approval: not approved"
    And the command succeeds

  # A step taken a moment ago has no restatement yet, and that is the state every taken step starts
  # in. So the line says what to do about it rather than reading as a fault.
  Scenario: Reading a restatement before the session wrote one says so, and names what to do
    Given the system listens on an address the tool can dial
    And the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      """
    When the operator takes step 1
    And the caller reads the restatement of step "1.1"
    Then standard output carries "this session wrote no restatement yet"
    And standard output carries "krewe exec"
    And the command succeeds

  # A step is named as <feature>.<number>, so 2.3 is step 3 of feature 2 and nothing else.
  Scenario: The caller takes step 3 of feature 2
    Given the system listens on an address the tool can dial
    And the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's feature "authentication"
    And the project's feature "payment"
    And the operator sets the path of feature 2 to:
      """
      ## 3. Refund a payment

      After
      """
    When the caller takes step "2.3"
    Then standard output carries "step 2.3 of house-bills is taken: Refund a payment"
    And the command succeeds

  # The way off the old form. A bare number was a whole step address before the path belonged to a
  # feature, so it is in somebody's notes and in their shell history. It names nothing now, and it
  # says so rather than being guessed at.
  Scenario: A bare step number is refused, and the refusal names the form and the features
    Given the system listens on an address the tool can dial
    And the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's feature "authentication"
    And the project's feature "payment"
    When the caller takes step "3"
    Then standard error says "name a step as <feature>.<number>, for example 2.3"
    And standard error says "1. authentication"
    And standard error says "2. payment"
    And the command fails

  # Refused even here, where there is only one feature it could mean. A guess that is right today is
  # wrong the moment a second feature is added, and it would be wrong silently.
  Scenario: A bare step number is refused on a project with exactly one feature
    Given the system listens on an address the tool can dial
    And the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's feature "authentication"
    And the operator sets the path of feature 1 to:
      """
      ## 1. Sign up
      """
    When the caller takes step "1"
    Then standard error says "name a step as <feature>.<number>, for example 2.3"
    And standard error says "1. authentication"
    And the command fails

  Scenario: A step token whose feature part names no feature is refused, naming the number
    Given the system listens on an address the tool can dial
    And the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's feature "authentication"
    When the caller takes step "9.1"
    Then standard error says "has no feature 9"
    And standard error says "it has 1"
    And the command fails

  Scenario: A step token with a part that is not a number is refused, naming the token
    Given the system listens on an address the tool can dial
    And the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's feature "authentication"
    When the caller takes step "1.one"
    Then standard error says "is not a step: the number after the full stop reads"
    And standard error says "1.one"
    And the command fails

  Scenario: A step token whose feature part is not a number is refused, naming the token
    Given the system listens on an address the tool can dial
    And the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's feature "authentication"
    When the caller takes step "one.1"
    Then standard error says "is not a feature number"
    And standard error says "one"
    And the command fails

  Scenario: The tool prints one line of refusal for a design nobody approved
    Given the system listens on an address the tool can dial
    And the project's design is "# Bills\n"
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      """
    When the caller takes step "1.1"
    Then standard error says "krewe design approve"
    And standard error says "nothing was started"
    And the command fails

  Scenario: Taking a step without saying which one is refused
    Given the system listens on an address the tool can dial
    When the caller takes a step without saying which one
    Then standard error says "usage: krewe step take"
    And the command fails

  # A step ends when somebody says what came of it. Nothing can see inside a container, so the result
  # is what a person wrote, and it is required: a step marked done with no result tells the next
  # session nothing.
  #
  # Done and stopped are the two words. A stop is how a step nobody will finish ends, and it carries
  # its reason in the same place a result goes, because there is nowhere else to read it.
  #
  # The word touches no session. The step and the session that took it are separate records, so the
  # row still says who took the work after the work is closed.

  Scenario: A step marked done says what came of it, and who closed it
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      ## 2. The store holds a project's design
      """
    And the operator took step 1
    When the operator finishes step 1 with "shipped as pull request 712, the brief reads back whole"
    Then step 1 is still done
    And the operator reads the path
    And step 1 says its result is "shipped as pull request 712, the brief reads back whole"
    And step 1 says "operator" closed it

  # The record of who did the work survives the word that closes it. A finish that cleared the
  # session would leave a step saying work happened and nothing saying who did it.
  Scenario: Finishing a step leaves the session and the take stamp where they were
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      """
    And the operator took step 1
    When the operator finishes step 1 with "shipped as pull request 712"
    Then step 1 still names the session that took it
    And the session that took it is untouched
    And 1 session was started
    And the model was asked 1 thing in all

  # Two words, and nothing else. An unknown word is refused rather than stored, the way an unknown
  # permission mode is, so one layer owns the vocabulary a step ends with.
  Scenario: Finishing a step with a word that is not done or stopped is refused
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      """
    And the operator took step 1
    When the operator finishes step 1 with the word "finished" and "shipped as pull request 712"
    Then the control plane refuses it as invalid
    And the refusal suggests "done"
    And the refusal suggests "stopped"
    And step 1 is still taken

  # The refusal this slice exists for. A step marked done with no result reads as work that happened
  # and says nothing about it, and the next session starts from that.
  Scenario: Finishing a step with no result is refused, and says why
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      """
    And the operator took step 1
    When the operator finishes step 1 with no result
    Then the control plane refuses it as invalid
    And the refusal suggests "nothing can see inside the container"
    And step 1 is still taken

  Scenario: A stopped step reads back stopped, carrying the reason
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      """
    And the operator took step 1
    When the operator stops step 1 with "the customer withdrew it"
    Then step 1 is still stopped
    And the operator reads the path
    And step 1 says its result is "the customer withdrew it"
    And step 1 says "operator" closed it

  # A stop runs no check and it refuses no unchecked step. Nothing ran against this step at all, and
  # stopping it is how a step nobody will finish ends.
  Scenario: Stopping a step nobody checked is not refused, and runs nothing
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      """
    And the operator took step 1
    When the operator stops step 1 with "the design moved on"
    Then step 1 is still stopped
    And the model was asked 1 thing in all

  Scenario: Finishing a step the path does not hold is refused, saying how many it has
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      ## 2. The store holds a project's design
      """
    When the operator finishes a step the path does not hold
    Then the control plane refuses it as not found
    And the refusal suggests "it has 2 steps"

  # Step 1 of one feature and step 1 of another are two steps, on the write that closes one as much
  # as on the write that takes it.
  Scenario: Finishing a step of one feature leaves the same number in another untouched
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's feature "authentication"
    And the project's feature "payment"
    And the operator sets the path of feature 1 to:
      """
      ## 1. Sign up
      """
    And the operator sets the path of feature 2 to:
      """
      ## 1. Checkout
      """
    When the operator finishes step 1 of feature 1 with "shipped as pull request 712"
    And the operator reads the path of feature 2
    Then step 1 is ready
    And step 1 says nothing came of it

  # What the whole call is for. A session on step 2 reads what step 1 produced, out of the path
  # document in its own working directory, because nothing else can tell it.
  Scenario: A session dispatched after a step is done reads what that step produced
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      ## 2. The store holds a project's design
      """
    And the operator finishes step 1 with "shipped as pull request 712, the brief reads back whole"
    When the operator dispatches "hello" to the project
    Then the session's path file carries "state: done"
    And the session's path file carries "result: shipped as pull request 712, the brief reads back whole"
    And the session's path file carries "### 2. The store holds a project's design\nmilestone: no milestone\nstate: ready"

  # These scenarios run the command line tool as a caller runs it: its own process, its own standard
  # output, its own exit status.

  Scenario: The caller marks a step done and the path reads it back
    Given the system listens on an address the tool can dial
    And the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      """
    When the caller marks step "1.1" done with "shipped as pull request 712"
    Then standard output carries "step 1.1 of house-bills is done: shipped as pull request 712"
    And the caller reads the path
    And standard output carries "done"

  # The operator's next command, under the line saying this one closed. It names a step and starts
  # nothing: the take is still typed.
  Scenario: Marking a step done prints the step that is next
    Given the system listens on an address the tool can dial
    And the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      ## 2. The store holds a project's design
      """
    When the caller marks step "1.1" done with "shipped as pull request 712"
    Then standard output carries "step 1.1 of house-bills is done"
    And standard output carries "next: step 2"
    And 0 sessions were started

  # The last step of a path closes and there is nothing to start. The line says so rather than naming
  # a step nobody may take.
  Scenario: Marking the last step done says nothing is next
    Given the system listens on an address the tool can dial
    And the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      """
    When the caller marks step "1.1" done with "shipped as pull request 712"
    Then standard output carries "next: nothing, every step is taken or waiting"

  # The two words take the same arguments in the same order, so the two are one thing to learn.
  Scenario: The caller stops a step and the reason reads back
    Given the system listens on an address the tool can dial
    And the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. The store holds a project's brief
      """
    When the caller stops step "1.1" with "the customer withdrew it"
    Then standard output carries "step 1.1 of house-bills is stopped: the customer withdrew it"
    And the operator reads the path
    And step 1 says its result is "the customer withdrew it"

  Scenario: Marking a step done without saying what came of it is refused
    Given the system listens on an address the tool can dial
    When the caller marks step "1.1" done without saying what came of it
    Then standard error says "usage: krewe step done"
    And the command fails

  # A project delivers several features at the same time. A website runs an authentication feature
  # and a payment feature at once: authentication ships sign up, then sign in, then reset, and
  # payment ships checkout, then refunds. Two paths, and neither waits for the other. A feature is
  # where each narrowed part of the project lives.
  #
  # A feature carries no design, no contracts document and no approval. Those belong to the project,
  # so gate 1 reads the project's design whichever feature a step sits in.

  Scenario: Adding two features to a project gives them the numbers 1 and 2
    When the operator adds the feature "authentication"
    And the operator adds the feature "payment"
    And the operator reads the features
    Then the features read 1, 2 in that order
    And feature 1 is titled "authentication"
    And feature 2 is titled "payment"

  # Feature numbers restart in each project, which is what makes the number the thing a person types.
  Scenario: A second project starts its features at one again
    Given a project named "the-website" beside it
    When the operator adds the feature "authentication"
    And the operator adds the feature "payment"
    And the operator adds the feature "checkout" to the project beside it
    And the operator reads the features of the project beside it
    Then the features read 1 in that order
    And feature 1 is titled "checkout"

  # The operator never chooses the number, so two adds at one moment cannot take the same one.
  Scenario: The number comes from the system and not from the caller
    When the operator adds the feature "authentication"
    Then the feature that was added took number 1
    And the feature that was added carries an identifier of its own

  Scenario: A feature with no title is refused
    When the operator adds a feature with no title
    Then the control plane refuses it as invalid
    And the operator reads the features
    And the project holds no feature

  Scenario: A fresh feature reads back open with an empty intention
    When the operator adds the feature "authentication"
    And the operator reads the features
    Then feature 1 is open
    And feature 1 narrows to ""

  Scenario: An intention reads back on the feature
    Given the project's feature "authentication"
    When the operator sets feature 1's intention to "sign up, sign in, reset, sessions"
    And the operator reads the features
    Then feature 1 narrows to "sign up, sign in, reset, sessions"

  # One line, because the intention is read beside the title in a listing. The rest goes in the
  # design.
  Scenario: An intention of two lines is refused
    Given the project's feature "authentication"
    When the operator sets feature 1's intention to "sign up\nsign in"
    Then the control plane refuses it as invalid
    And the operator reads the features
    And feature 1 narrows to ""

  # No length cap refuses text a person wrote. The line is kept whole and a person decides.
  Scenario: An intention past 200 characters warns and is kept
    Given the project's feature "authentication"
    When the operator sets feature 1's intention to 250 characters
    Then the feature write warns "over the 200 characters mark"
    And the operator reads the features
    And feature 1's intention is 250 characters

  Scenario: The features of a project that does not exist are refused
    When the operator reads the features of a project that does not exist
    Then the control plane refuses it as not found

  # A design session names the features it is about to write paths for, and naming one grants it
  # nothing it could not reach by dispatching.
  Scenario: A design session may add a feature
    When the driver adds the feature "authentication"
    And the operator reads the features
    Then the features read 1 in that order

  Scenario: Deleting a project leaves no feature of it anywhere
    Given the project's feature "authentication"
    When the operator deletes the project
    Then no feature of that project is left anywhere
    And reading its features is refused as not found

  # These scenarios run the command line tool as a caller runs it: its own process, its own standard
  # output, its own exit status.

  Scenario: A project with no feature tells the caller how to add one
    Given the system listens on an address the tool can dial
    When the caller reads the features
    Then standard output carries "has no feature yet"
    And standard output carries "krewe feature add"
    And the command succeeds

  Scenario: The caller adds a feature and reads it back
    Given the system listens on an address the tool can dial
    When the caller adds the feature "authentication"
    Then standard output carries "has feature 1: authentication"
    And standard output carries "krewe feature intention"
    And the caller reads the features
    And standard output carries "authentication"
    And standard output carries "open"
    And the command succeeds

  Scenario: Two features print in number order
    Given the system listens on an address the tool can dial
    And the project's feature "authentication"
    And the project's feature "payment"
    When the caller reads the features
    Then standard output lists 2 features in number order
    And the command succeeds

  # The count is of the feature's own steps. Counting the project's steps instead would print one
  # project wide number against every feature of it and say nothing about any of them.
  Scenario: A feature counts the steps under it, and holds none yet
    Given the system listens on an address the tool can dial
    And the project's feature "authentication"
    When the caller reads the features
    Then standard output carries "0/0"
    And the command succeeds

  Scenario: Each feature counts its own steps and not the project's
    Given the system listens on an address the tool can dial
    And the project's feature "authentication"
    And the project's feature "payment"
    And the operator sets the path of feature 1 to:
      """
      ## 1. Sign up
      ## 2. Sign in
      """
    And the operator sets the path of feature 2 to:
      """
      ## 1. Checkout
      """
    When the caller reads the features
    Then standard output carries "0/2"
    And standard output carries "0/1"
    And the command succeeds

  Scenario: Setting an intention shows it on the next reading
    Given the system listens on an address the tool can dial
    And the project's feature "authentication"
    When the caller sets feature 1's intention to "sign up, sign in, reset, sessions"
    Then standard output carries "narrows to: sign up, sign in, reset, sessions"
    And the caller reads the features
    And standard output carries "sign up, sign in, reset, sessions"
    And the command succeeds

  # A number nobody wrote and a number one past the end read the same to whoever typed it.
  Scenario: A feature number that names no feature is refused, naming the numbers that exist
    Given the system listens on an address the tool can dial
    And the project's feature "authentication"
    And the project's feature "payment"
    When the caller sets feature 9's intention to "sign up, sign in"
    Then standard error says "has no feature 9"
    And standard error says "it has 1, 2"
    And the command fails

  Scenario: Setting an intention without saying which feature is refused
    Given the system listens on an address the tool can dial
    When the caller sets an intention without saying which feature
    Then standard error says "usage: krewe feature intention"
    And the command fails

  # A feature is closed when the operator says so. It is closed while steps under it are ready or
  # taken all the time: the work moved on, or somebody abandoned it, and the operator is the one who
  # knows which. So the control plane warns about each step still open and writes the state anyway. A
  # refusal here would make somebody finish every step of a feature nobody works on before the record
  # of the work could stop growing.
  #
  # The warning also says what closing costs. A closed feature leaves .krewe/path.md, so a session in
  # the project stops reading its path, and a person who closes a feature and then finds a session
  # with nothing to read would take the silence for a fault.
  #
  # Closing touches no step and no milestone. A closed feature keeps its whole path, and krewe feature
  # open is the way back from both closed words.

  Scenario: Closing a feature with a taken step warns about that step and closes it anyway
    Given the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. Sign up
      ## 2. Sign in
      """
    And the operator takes step 1
    When the operator closes feature 1
    Then the feature write warns "step 1 is taken"
    And the operator reads the features
    And feature 1 reads as "done"

  # A person who closes a feature and finds a session idle reads the silence as a fault, so the write
  # says what it takes away.
  Scenario: The warning says the feature leaves the path document a session reads
    Given the project's feature "authentication"
    When the operator closes feature 1
    Then the feature write warns ".krewe/path.md"

  # The store keeps the word it is given, so the vocabulary is guarded in one place and the refusal
  # names every word that works.
  Scenario: A state word outside the three is refused, and the refusal names the three
    Given the project's feature "authentication"
    When the operator sets feature 1's state to "finished"
    Then the control plane refuses it as invalid
    And the refusal says "open, done, stopped"
    And the operator reads the features
    And feature 1 is open

  # A state is not a delete.
  Scenario: The steps of a closed feature read back unchanged
    Given the project's path is:
      """
      ## 1. Sign up
      ## 2. Sign in
      """
    And the operator takes step 1
    And the path as it stands is written down
    When the operator closes feature 1
    And the operator reads the path
    Then the path reads back as it was written down

  Scenario: The steps of a stopped feature read back unchanged
    Given the project's path is:
      """
      ## 1. Sign up
      ## 2. Sign in
      """
    And the path as it stands is written down
    When the operator stops feature 1
    And the operator reads the path
    Then the path reads back as it was written down

  # Reopening costs nothing and starts nothing, so it warns about nothing.
  Scenario: Reopening a closed feature writes the word open, and warns nothing
    Given the project's feature "authentication"
    And the operator closes feature 1
    When the operator opens feature 1 again
    Then the feature write warns nothing
    And the operator reads the features
    And feature 1 is open

  Scenario: Reopening a stopped feature writes the word open
    Given the project's feature "authentication"
    And the operator stops feature 1
    When the operator opens feature 1 again
    And the operator reads the features
    Then feature 1 is open


  # A closed feature leaves the file, which is what keeps it from growing with every finished feature.
  # A session reading mostly history is reading the wrong document.
  Scenario: A closed feature is left out of the path the session reads
    Given the project's feature "authentication"
    And the project's feature "payment"
    And the operator sets the path of feature 1 to:
      """
      ## 1. Sign up
      """
    And the operator sets the path of feature 2 to:
      """
      ## 1. Checkout
      """
    And the operator closes feature 1
    When the operator dispatches "hello" to the project
    Then the session's path file carries "### 1. Checkout"
    And the session's path file does not carry "Sign up"
    And the session's path file does not carry "# 1. authentication"

  # A file that exists and says nothing costs a read, and a project whose only path is closed has
  # nothing to say.
  Scenario: A project whose open features hold no step has no path file
    Given the project's design is "# Bills\n"
    And the project's feature "authentication"
    And the operator sets the path of feature 1 to:
      """
      ## 1. Sign up
      """
    And the operator closes feature 1
    When the operator dispatches "hello" to the project
    Then the session has no path file
    And the session's memory file does not carry ".krewe/path.md"
  # A closed feature leaves the file, and reopening brings it back on the next dispatch.
  Scenario: A reopened feature is back in the path the session reads
    Given the project's path is:
      """
      ## 1. Sign up
      """
    And the operator closes feature 1
    And the operator opens feature 1 again
    When the operator dispatches "hello" to the project
    Then the session's path file carries "### 1. Sign up"

  # These scenarios run the command line tool as a caller runs it: its own process, its own standard
  # output, its own exit status.

  Scenario: The caller closes a feature that holds a taken step, and reads what it cost
    Given the system listens on an address the tool can dial
    And the project's design is "# Bills\n"
    And the operator approved the project's design
    And the project's path is:
      """
      ## 1. Sign up
      """
    And the operator takes step 1
    When the caller closes feature 1
    Then standard output carries "is done"
    And standard output carries "step 1 is taken"
    And standard output carries ".krewe/path.md"
    And the command succeeds

  Scenario: The caller closes a feature and the listing shows the word done
    Given the system listens on an address the tool can dial
    And the project's feature "authentication"
    When the caller closes feature 1
    And the caller reads the features
    Then standard output carries "done"
    And the command succeeds

  # The reason is printed back and never stored. A feature carries no result column, because what came
  # of the work is on its steps.
  Scenario: The caller stops a feature and the reason is printed back and not kept
    Given the system listens on an address the tool can dial
    And the project's feature "authentication"
    When the caller stops feature 1 saying "the customer withdrew it"
    Then standard output carries "is stopped: the customer withdrew it"
    And the caller reads the features
    And standard output carries "stopped"
    And standard output does not carry "the customer withdrew it"
    And the command succeeds

  Scenario: The caller opens a feature again
    Given the system listens on an address the tool can dial
    And the project's feature "authentication"
    And the caller closes feature 1
    When the caller opens feature 1 again
    Then standard output carries "is open again"
    And the caller reads the features
    And standard output carries "open"
    And the command succeeds

  # A number nobody wrote and a number one past the end read the same to whoever typed it.
  Scenario: Closing a feature number that names no feature is refused, naming the numbers that exist
    Given the system listens on an address the tool can dial
    And the project's feature "authentication"
    When the caller closes feature 9
    Then standard error says "has no feature 9"
    And standard error says "it has 1"
    And the command fails

  Scenario: Closing a feature without saying which one is refused
    Given the system listens on an address the tool can dial
    When the caller closes a feature without saying which one
    Then standard error says "usage: krewe feature done"
    And the command fails

  Scenario: Stopping a feature without saying why is refused
    Given the system listens on an address the tool can dial
    And the project's feature "authentication"
    When the caller stops feature 1 and says nothing more
    Then standard error says "usage: krewe feature stop"
    And the command fails
