-- The scope goes, and the check then binds every step again, which is what it did the day it
-- shipped.
--
-- A step of a project with no proof command cannot finish after this, the way it could not before.
-- The column holds the answer nowhere else, so this drops it rather than moving it.
--
-- The red run requirement this migration cleared off those steps does not come back, because nothing
-- recorded which rows it cleared. An operator who rolls back and wants that rule over a step in
-- flight again takes the step again, which is what binds one.
alter table feature_steps drop column if exists check_required;
