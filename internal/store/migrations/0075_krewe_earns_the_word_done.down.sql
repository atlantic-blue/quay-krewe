-- The trust record goes, and every count with it. The seven columns hold it nowhere else in the
-- store, so this drops the record rather than moving it.
--
-- A project that earned level 1 is back at level 0 when this is applied again, so krewe closes no
-- step and the operator says every word. That is where the system starts, and it costs the operator
-- the run of agreements rather than any work.
--
-- Nothing else moves. A step that is done stays done, and who closed it stays on the row: closed_by
-- belongs to migration 0070 and is dropped there.
alter table feature_steps drop column if exists operator_agreed;
alter table project_designs drop column if exists trust_disagreements;
alter table project_designs drop column if exists trust_agreements;
alter table project_designs drop column if exists trust_offered;
alter table project_designs drop column if exists trust_run;
alter table project_designs drop column if exists trust_threshold;
alter table project_designs drop column if exists trust_level;
