-- Every contracts document in the system goes with this column. The text exists nowhere else: it is
-- not on `projects`, it is not in `contexts`, and the file a session reads is written from this
-- column rather than kept beside it.
--
-- The design body and its approval are untouched. A design whose contracts are dropped is still the
-- design the operator approved.
alter table project_designs drop column if exists contracts;
