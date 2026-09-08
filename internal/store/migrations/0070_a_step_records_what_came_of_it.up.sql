-- Who spoke the word that finished a step: 'operator' or 'krewe'. It is empty while the step is
-- ready or taken.
--
-- The result and the finish stamp are already on the table, from the migration that created it. This
-- is the column the write needs that was not there: a step that reads as done says nothing about who
-- closed it, and krewe closes a step itself once it earns the word.
--
-- The column beside it in the design, operator_agreed, waits for the trust record. It is a statement
-- about a verdict, and nothing runs a verdict yet.
alter table feature_steps add column if not exists closed_by text not null default '';
