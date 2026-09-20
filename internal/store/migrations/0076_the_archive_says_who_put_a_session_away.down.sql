-- The reason goes and every session stays. The column holds this nowhere else, so this drops the
-- record rather than moving it.
--
-- A session archived while the column existed reads the empty string when this is applied again,
-- which is the same thing a session archived before it ever existed reads: nobody can say why it
-- went. Nothing else on the row moves, so a session that is put away stays put away with its stamp.
alter table sessions drop column if exists archived_reason;
