-- The contracts this step builds, one identifier per line.
--
-- A contract identifier is a string the operator wrote in the path document. Nothing here checks that
-- it names a contract the project's contracts document holds: krewe never reads that document, and
-- the session opens it for itself.
alter table feature_steps add column if not exists contracts text not null default '';

-- What part of each contract is this step's, one line per contract, reading `<identifier>:
-- <sentence>`. The sentence says which part this step builds and which part waits for a later step.
--
-- It is text beside the contracts rather than a table of its own, because it is one sentence per
-- contract of one step and nothing reads it except the take text.
--
-- The path write refuses a document whose scope names an identifier the contracts column does not
-- name, so the two columns cannot disagree about which contracts a step builds.
alter table feature_steps add column if not exists contract_scope text not null default '';
