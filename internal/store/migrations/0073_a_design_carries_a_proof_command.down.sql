-- What one scenario run looks like goes, and every project's own settings with it. The three columns
-- hold that nowhere else in the store, so this drops the record rather than moving it.
--
-- A project that set its own command is back at whatever the columns above default to when this is
-- applied again, which is a project that proves nothing until somebody sets one.
--
-- No design moves. The columns say how a run is made and never what the design says, so dropping them
-- leaves every approval exactly where it is.
alter table project_designs drop column if exists proof_timeout_seconds;
alter table project_designs drop column if exists proof_count_pattern;
alter table project_designs drop column if exists proof_command;
