-- The interview goes, and the stages that were there sit at the positions they held before.
--
-- Every interview row is removed, including one an operator wrote after the migration ran. The stage
-- does not exist below this version, so a row holding a page would be a row no name in the code can
-- read. The page itself is gone: nothing else recorded it, and an operator who rolls back and wants
-- those answers asks the questions again.

delete from project_design_stages where stage = 'interview';

update project_design_stages
set position = position - 1
where position > 0;
