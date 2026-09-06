-- One row per milestone of one feature's path.
--
-- A milestone groups the steps of a feature, so a reader sees what a run of steps adds up to. It is
-- written by the path write, from the same document that writes the steps, because one document
-- carries the whole path of one feature and two documents would let the two disagree.
--
-- The key is (feature, number) rather than an identifier of its own. Nothing points at a milestone:
-- a step names its milestone by number, and a number restarts in each feature.
--
-- There is no state column. What a milestone reached is counted from the steps under it, and a
-- second answer to that question is a second answer that can be wrong.
create table if not exists milestones (
    feature    text        not null references features (id) on delete cascade,
    -- Where in the feature, counting from one. Numbers restart in each feature and need not be
    -- contiguous, so a feature may hold milestones 1, 2 and 5.
    number     integer     not null,
    title      text        not null,
    -- One line saying what the milestone is for.
    intention  text        not null default '',
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    primary key (feature, number)
);

-- Which milestone of the feature a step belongs to. Zero means the step belongs to no milestone, and
-- zero is never a row in `milestones`.
--
-- It is a number rather than a reference, because a milestone is keyed by (feature, number) and the
-- path write replaces both tables in one transaction: a step and its milestone are written together
-- or neither is.
alter table feature_steps add column if not exists milestone integer not null default 0;
