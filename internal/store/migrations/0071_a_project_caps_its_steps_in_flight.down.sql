-- The cap goes and the fan out is unbounded again. Every project's own number goes with it: the cap
-- is held nowhere else, so a project that raised or lowered it is back at whatever the column above
-- defaults to when this is applied again.
--
-- No step moves. The column governs the next take and never a session that already runs, so dropping
-- it leaves every step in state `taken` exactly where it is.
alter table project_designs drop column if exists steps_in_flight_cap;
