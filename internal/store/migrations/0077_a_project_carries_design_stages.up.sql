-- The six stages a project is designed in, before anything is built: one row per stage.
--
-- A project used to start from its design document, which is one text covering everything at once, so
-- the data model and the architecture were written in the same breath as what a person sees. These
-- six rows put an order on it: discovery, stories, design_system, mockups, data_model, architecture.
-- A stage cannot be written until the stage before it is approved, so the data model can never be
-- written before the stories.
--
-- The rows are made on the first write to a stage rather than when the project is made. A project
-- that holds no row here is a project designed the way every project was designed until today, and it
-- keeps that behaviour.
--
-- Approval is a statement about one text, and version is what holds that statement to it: a write
-- increases version and approved_version stays where it was, so a stage reads approved only while the
-- two are equal. The same write clears the approval of every later stage, because a stage that
-- changed is a stage the later ones were agreed under.
create table if not exists project_design_stages (
    id               text        primary key,
    project          text        not null references projects (id) on delete cascade,
    -- One of the six names. The order they are written in is position, and both stores read one list,
    -- so a name outside the six is refused before it reaches here.
    stage            text        not null,
    -- Where in the order, counting from zero, so 0 is discovery and 5 is architecture. It is a column
    -- rather than a lookup because every read of this table is in that order, and the rule that
    -- refuses a write reads the stages below a position.
    position         integer     not null,
    body             text        not null default '',
    -- The structured artifact a stage carries beside its prose, such as the flows a mockup stage is
    -- played from. Null when the stage carries none: an empty artifact is no artifact, and an empty
    -- string is not json.
    artifact         jsonb,
    -- Where the artifact is published, when it is published somewhere a person opens.
    artifact_url     text,
    -- Increased on each write to the body or the artifact.
    version          integer     not null default 1,
    -- The version the operator approved. Approval holds only while it equals version, so a write
    -- takes it away without a second statement having to remember to.
    approved_version integer,
    approved_at      timestamptz,
    -- Only discovery may be skipped, and a skipped stage counts as satisfied by the rule that
    -- refuses a write. Nothing writes this column yet: the command line that does is its own step.
    skipped          boolean     not null default false,
    created_at       timestamptz not null default now(),
    updated_at       timestamptz not null default now(),
    -- One row per stage per project, which is what makes a write an upsert on the name.
    unique (project, stage)
);

-- One project's stages, which is the only ordinary read there is. The primary key answers the other
-- one, which is a read by identifier.
create index if not exists project_design_stages_project_idx on project_design_stages (project);
