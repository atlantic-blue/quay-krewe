-- What krewe's own run of the scenario reported: the verdict, how many scenarios ran, what the run
-- printed, and when it ran.
--
-- The four columns are one record and they are written together, whatever the run said. A failing run
-- is a record and not a gap: the moment is stamped on a failing run too, so the gate that refuses a
-- finish before anybody read a verdict opens either way.
--
-- Nothing here is a judgement about the work. Krewe reads an exit status and a count, and the
-- operator reads the scenario when the operator approves the design.
alter table feature_steps add column if not exists proof_state text not null default 'unproven';

-- How many scenarios the last run reported, read out of the output with the project's own pattern.
--
-- Zero means nothing ran, and zero never passes. A name filter that matches nothing prints success in
-- most runners, so a check that read the exit status alone would report a passing verdict on a
-- scenario that never ran.
alter table feature_steps add column if not exists proof_scenarios_run integer not null default 0;

-- The last 4,000 characters of the last run. When the output was cut, the first line says how much
-- was dropped.
--
-- It is data and never code. It carries whatever the run printed, it is stored and shown to the
-- operator, and nothing executes it and nothing renders it into a memory file.
alter table feature_steps add column if not exists proof_output text not null default '';

-- When the last run ran, and null until one does. It is what says a step was checked at all, which is
-- a different thing from a step whose check failed.
alter table feature_steps add column if not exists proof_ran_at timestamptz;
