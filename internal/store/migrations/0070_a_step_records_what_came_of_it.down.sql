-- Who closed each step goes with this column. It exists nowhere else: the path document a session
-- reads is written from these rows rather than kept beside them.
--
-- Every other column is untouched. A step whose closer is dropped still reads as done, still carries
-- what came of it, and still names the session that took it.
alter table feature_steps drop column if exists closed_by;
