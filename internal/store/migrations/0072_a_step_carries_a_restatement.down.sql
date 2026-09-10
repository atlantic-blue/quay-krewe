-- The restatement goes, and every approval with it. What each session wrote about its step is held
-- nowhere else in the store, so this drops the record rather than moving it.
--
-- The session's own memory file still carries the text it wrote, under the restatement mark, so a
-- step whose session is still running is restated again on that session's next exec once the columns
-- are back.
--
-- No step moves. The columns say what a session understood and never what state the step is in, so
-- dropping them leaves every step exactly where it is.
alter table feature_steps drop column if exists restatement_approved_at;
alter table feature_steps drop column if exists restatement_approved;
alter table feature_steps drop column if exists restated_at;
alter table feature_steps drop column if exists restatement;
