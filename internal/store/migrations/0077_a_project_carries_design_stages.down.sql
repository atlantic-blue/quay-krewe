-- Every design stage in the system goes with this table. They exist nowhere else: they are not on
-- `projects`, they are not on `project_designs`, and no file on the host holds a copy. A project
-- rolled back to before this migration is a project designed by its design document alone, which is
-- how every project was designed until today.
drop table if exists project_design_stages;
