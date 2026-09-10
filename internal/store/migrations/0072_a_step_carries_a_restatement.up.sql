-- What the session wrote about a step before it built anything, and the operator's word about that
-- text.
--
-- The session is given the step and told to write no code. What it writes back is the evidence that
-- it read the step the way the step was meant, and the operator reads that before any code exists.
--
-- The four columns are one thing and they move together. `restatement` is the text, `restated_at`
-- says when it last changed, and the two approval columns say whether anybody agreed to it.
alter table feature_steps add column if not exists restatement text not null default '';

-- When the session last wrote one. Null until it writes anything, which is the state every taken
-- step starts in.
alter table feature_steps add column if not exists restated_at timestamptz;

-- The operator's word about this exact text, and never about the step. A second restatement clears
-- it, so a text nobody read cannot inherit the approval of the text before it.
alter table feature_steps add column if not exists restatement_approved boolean not null default false;

-- When that word was spoken. Null while the approval is false.
alter table feature_steps add column if not exists restatement_approved_at timestamptz;
