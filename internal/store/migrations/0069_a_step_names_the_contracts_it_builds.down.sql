-- The contracts each step builds, and the scope of each one, go with these two columns. They exist
-- nowhere else: the path document a session reads is written from these rows rather than kept beside
-- them, and no file on the host holds a copy.
--
-- Every other column is untouched. A path whose contracts are dropped is still the same path, and a
-- step somebody took still names the session that holds it.
alter table feature_steps drop column if exists contract_scope;

alter table feature_steps drop column if exists contracts;
