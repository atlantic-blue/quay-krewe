-- The record of the run that was seen to fail goes, and the gate that reads it goes with it. The two
-- columns hold it nowhere else in the store, so this drops the record rather than moving it.
--
-- A step in flight then finishes on a verdict alone, which is what every step did before today.
-- Nothing else on the row moves: the verdict, the restatement and the word spoken over it say
-- different things, and a step that is done stays done.
alter table feature_steps drop column if exists red_run_at;
alter table feature_steps drop column if exists red_run_scenarios;
