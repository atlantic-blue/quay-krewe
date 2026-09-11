-- The record of what krewe's own run reported goes, and every step's verdict with it. The four
-- columns hold it nowhere else in the store, so this drops the record rather than moving it.
--
-- A step that was checked reads as unproven when this is applied again, which is a step nobody has
-- run the scenario for. Running it again is one command and costs no model tokens.
--
-- Nothing else on the row moves. The verdict says what a run reported and never what the step is, so
-- a step that is done stays done, and the restatement and its approval stay where they are.
alter table feature_steps drop column if exists proof_ran_at;
alter table feature_steps drop column if exists proof_output;
alter table feature_steps drop column if exists proof_scenarios_run;
alter table feature_steps drop column if exists proof_state;
