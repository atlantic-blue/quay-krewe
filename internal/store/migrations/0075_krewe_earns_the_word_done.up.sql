-- The trust record: how often the operator agreed with what krewe's own run of a scenario reported.
--
-- The word done starts with the operator and moves to krewe as krewe earns it. Nothing here moves it.
-- These columns count the agreements, and a later change reads the count.
--
-- The count is a run of consecutive agreements and never a ratio. One disagreement starts the run
-- again, because a ratio over a long record drifts upward and stops saying anything about the last
-- ten steps.

-- Level 0: krewe checks and the operator says done. Level 1: krewe closes a step its own check
-- passed. The column never holds another number: 0 and 1 are the whole ladder.
alter table project_designs add column if not exists trust_level integer not null default 0;

-- The run of agreements that earns an offer of the next level. Five is a guess and nothing measured
-- it, which is why the project sets its own.
alter table project_designs add column if not exists trust_threshold integer not null default 5;

-- Consecutive agreements since the last disagreement or the last level change.
alter table project_designs add column if not exists trust_run integer not null default 0;

-- True after krewe offers the next level, until the operator answers. Krewe never raises its own
-- level, so the offer stands until somebody accepts it.
alter table project_designs add column if not exists trust_offered boolean not null default false;

-- Every agreement and every disagreement this project recorded. The run above says how things stand
-- now, and these two say what the whole record is.
alter table project_designs add column if not exists trust_agreements integer not null default 0;
alter table project_designs add column if not exists trust_disagreements integer not null default 0;

-- Whether the operator's word matched krewe's last verdict: 'yes', 'no', or empty while nothing is
-- decided.
--
-- It is read from the row and never asked. Done after a passing check is 'yes', stopped after a
-- failing check is 'yes', and anything else is 'no'. A question with an obvious answer costs one more
-- keystroke and buys nothing.
--
-- The column beside it, closed_by, is already here: migration 0070 added it when a step first
-- recorded what came of it.
alter table feature_steps add column if not exists operator_agreed text not null default '';
