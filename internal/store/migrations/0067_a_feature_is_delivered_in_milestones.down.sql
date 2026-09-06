-- Every milestone in the system goes with this table, and which milestone each step belonged to goes
-- with the column. They exist nowhere else: no file on the host holds a copy, and the path document
-- a session reads is written from these rows rather than kept beside them.
--
-- The steps are untouched. A path whose milestones are dropped is still the same path.
alter table feature_steps drop column if exists milestone;

drop table if exists milestones;
