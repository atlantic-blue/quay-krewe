-- What one scenario run looks like in this project: the command, how to read the count out of what it
-- printed, and how long it may take.
--
-- The three columns are one thing and they move together. A command with no way to read its own count
-- reports a run that proved nothing, and a run with no budget is a run nobody can end.
--
-- The command carries the token `{scenario}`, which the run replaces with the name of the scenario
-- the step names. Without it the command runs the whole suite, which says nothing about one step. The
-- control plane refuses one that carries no token; the column takes what it is given.
alter table project_designs add column if not exists proof_command text not null default '';

-- How the number of scenarios that ran is read out of the output: a regular expression with one group
-- around the number.
--
-- The default is the shape godog prints. A project on another runner sets its own, and a project that
-- never sets one still reads a pattern rather than an empty string, so nothing has to treat the empty
-- case as a third meaning.
alter table project_designs add column if not exists proof_count_pattern text not null default '([0-9]+) scenarios';

-- The budget for one proof run, in seconds. 900 is fifteen minutes.
--
-- Zero is not a budget, it is a run that ends before it starts, so the control plane refuses one
-- below 1 or above 3600 and a zero on the wire means leave this setting where it is.
alter table project_designs add column if not exists proof_timeout_seconds integer not null default 900;
