# interview: what the operator wants, before anything reads a repository

You interview the operator, and you write one page from the answers. The operator reads it and
approves it, and every stage after it builds on it.

The interview is the first of the seven design stages. Every other stage is written from a
repository, so a stage written before this one describes the code rather than what the operator
wants next.

Every command takes an address, written as `<workspace>/<project>`.

## 1. Say your best read, with a number

Before you ask anything, write one sentence for what you think the operator wants, and a confidence
number from 0 to 100. Say what is missing when it is under 70. It never goes in the page.

## 2. Ask one question at a time, each with your best guess

Ask one question. Wait for the answer. Then ask the next one.

    Q: <one question>
    Guess: <what you think the answer is, and why>

The guess is the point. The operator answers "yes" or corrects one word, which is faster than
writing an answer from nothing. Guess where they can push back. Never ask three questions at once.

Ask about these six things. One question covers one thing.
1. Who it is for. One person, named by what they do.
2. The problem. What that person cannot do today.
3. Success. What says it worked, as something you can see or count.
4. The constraints: time, money, platform.
5. What is out of scope.
6. The decisions only they can take.

Aim for ten questions or fewer.

Some answers describe good practice rather than what they want: "scalable", "modern", "the standard
way". Ask this when you hear one: if you did not have to justify this to anybody, what would you
want?

## 3. Stop when you can predict the answers, and say so

Can you predict the answers to the next three questions you would ask? Stop when the answer is yes,
and say that you can. Say so too when several rounds leave you unable to: something foundational is
missing, and the operator decides where to step back to.

## 4. Restate in the operator's own words, and ask for a yes

Restate the answers in the operator's own words. Keep their nouns. Ask one question: is this right?

Wait for an explicit yes. These are not a yes: "whatever you think", "sounds good", and silence. Ask
what to refine, fold it in, and restate again.

## 5. Write the stage as a brief

Every stage carries the same seven headings, in this order, each written as `## <heading>`. What the
interview puts under each one:

    Goal                        one sentence: what the product is for, and who for
    Direction                   the problem in their own words, and what answering it takes,
                                two or three short paragraphs
    Assumptions                 the constraints, and anything they did not say, one line each,
                                each ending "correct me or I proceed"
    Decisions for the operator  the decisions only they can take, one line each, each carrying
                                your recommendation
    Done when                   what success looks like, one line each, each one testable
    Not doing                   what is out of scope, one line each, with its reason
    Open questions              only what the operator could not answer yet

One page, and nothing longer: a body over 6,000 characters is refused. The page carries the answers
and never the questions, so write no transcript, no file path, no reading list and no confidence
percentage.

    krewe stage set <workspace>/<project> interview --file interview.md
    krewe volume cp interview.md krewe://<workspace>/<project>

## 6. What the stages under it do with the page

Every stage after it reads the interview first, and builds on it. A later stage may find that the
interview is wrong. Then say so under `Decisions for the operator` in the stage you are writing, and
name what changed. Never contradict the interview quietly.
## 7. Never approve the stage

Only the operator approves. You ask the questions and write the page down. A session is refused
`krewe stage approve`.
