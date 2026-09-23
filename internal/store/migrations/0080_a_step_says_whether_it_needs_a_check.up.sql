-- Whether the word done on a step waits for a verdict krewe read: true where the project said how
-- one scenario of it is run at the moment of the take, and false where it said nothing.
--
-- Both gates stand on a proof command. Krewe runs that command to reach a verdict, so a project that
-- never set one has nothing for krewe to run: its steps could not be checked, and the word done then
-- refused every one of them. Two projects this system was building were in exactly that state, with
-- the work merged and the step unable to close.
--
-- So the default is false, which is what every row this statement lands on reads, and the take writes
-- what the project asks for from now on. The same column decides whether the take text asks the
-- session to restate the step first: a restatement nobody can act on through a check gates nothing.
alter table feature_steps add column if not exists check_required boolean not null default false;

-- The steps the gates stranded, freed. A session is holding each of them, their project says nothing
-- about how a scenario of it runs, and no run they could make would let them close.
--
-- Read at the state and at the project, and never at the moment: a step of a project that does prove
-- its steps keeps the red run rule it was taken under, because that step can still meet it. A step
-- nobody took carries the default and needs no statement.
update feature_steps s
set check_required = false, red_run_required = false
where s.state = 'taken'
  and not exists (
    select 1
    from features f
    join project_designs d on d.project = f.project
    where f.id = s.feature
      and d.proof_command <> '');
