-- The scope goes, and the rule then binds every step again, which is what it did the day it shipped.
--
-- A step in flight from before the rule cannot finish after this, the way it could not before. The
-- column holds the answer nowhere else, so this drops it rather than moving it.
--
-- Nothing else on the row moves. The record of the run that went red, the verdict and the word
-- spoken over the restatement all say different things, and a step that is done stays done.
alter table feature_steps drop column if exists red_run_required;
