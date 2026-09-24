package flowmap

import _ "embed"

// PageHTML is skills/flow-map/index.html, as it is on disk.
//
// The page has one home, and this is it. The control plane serves these bytes at the stage whose
// screens they play, so nobody copies the file next to a flows.json and nobody publishes it to a
// web host. A copy would be a second page, and the one an operator opened would be whichever copy
// somebody remembered to update.
//
// The embed sits in this package rather than in the control plane because an embed pattern cannot
// name a directory above its own.
//
//go:embed index.html
var PageHTML []byte
