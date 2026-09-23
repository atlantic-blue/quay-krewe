-- The run that was seen to fail: how many scenarios it reported, and when it ran.
--
-- A test nobody saw fail proves nothing. It passes on code that is right, and it passes on an
-- assertion that is empty, and the two read the same from outside. So the word done reads this record
-- as well as the verdict: a step builds in two commits, the tests first with the run on them red, the
-- code second with the run on it green.
--
-- The two columns are one record and they are written together, by the same call that records a
-- verdict. Nothing else writes them, and no document a caller sends can set them.
--
-- A step in flight when this runs carries no record, whatever its session already saw. The tests are
-- run once more and the record lands, which is one command and spends no model tokens. Backfilling
-- the moment from the verdict would write a run that nobody made.
alter table feature_steps add column if not exists red_run_scenarios integer not null default 0;

-- When a run of this step's scenario was last seen to fail with at least one scenario in it, and null
-- until one is. The word done is refused while it is null.
--
-- It is stamped only where the run reported a scenario. A name filter that matches nothing, and a
-- command the shell cannot start, both report a failure that executed no test, so a run of zero
-- scenarios leaves this where it was.
alter table feature_steps add column if not exists red_run_at timestamptz;
