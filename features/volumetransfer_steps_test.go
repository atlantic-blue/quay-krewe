package features_test

import (
	"context"
	"path/filepath"

	"github.com/cucumber/godog"
)

// The step that puts the volume out of the command's reach.
//
// A volume is a directory on the machine that runs the sandboxes, and the system holds two paths to
// it: the one this process opens, and the one that machine uses. Under Kubernetes those are
// different paths, and only the control plane can open either. So a scenario moves the second one
// somewhere that does not exist. Every answer the tool is given then carries a path that reaches
// nothing, and a copy that opens a path instead of asking cannot work.
//
// Both rows of the outline take this road, including the one where the volume is on this machine.
// A row that skipped the harness would pass without saying anything.

// beyondTheCommand is the name of the directory a scenario says the sandboxes are under. It is never
// made, because nothing on this machine is meant to open it.
const beyondTheCommand = "on-another-machine"

func initializeVolumeTransferSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the volume is (somewhere the command cannot reach on its own filesystem|on the machine the command runs on)$`,
		func(ctx context.Context, where string) error {
			w := worldFrom(ctx)
			if where == "on the machine the command runs on" {
				w.storage.Host = w.storage.Dir
			} else {
				w.storage.Host = filepath.Join(w.home, beyondTheCommand)
			}
			// The control plane reads the layout when it is built, so the change reaches it on the way
			// up. The scenario asks for an address to dial after this, which is the new server's.
			return w.restart()
		})
}
