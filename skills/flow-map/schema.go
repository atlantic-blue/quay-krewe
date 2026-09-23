// Package flowmap carries the schema a flows.json is read against.
//
// The schema is the contract between the skill that writes a mockup and the control plane that
// keeps it. A session writes flows.json by reading schema.json; the mockups stage refuses an
// artifact the same file says is wrong. One file, read by both, so the two cannot drift.
//
// It is embedded rather than read from disk. The runtime image copies the skills in, but a control
// plane that is given no skills directory would then find no schema and validate nothing, and a
// check that quietly stops checking reads exactly like a check that passes.
package flowmap

import _ "embed"

// SchemaJSON is skills/flow-map/schema.json, as it is on disk.
//
//go:embed schema.json
var SchemaJSON []byte

// SchemaID is what the schema calls itself. A validator wants a name to resolve a reference
// against, and a refusal that names the file sends the reader to the rule they broke.
const SchemaID = "https://github.com/atlantic-blue/quay-krewe/skills/flow-map/schema.json"
