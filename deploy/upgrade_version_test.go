package deploy

import (
	"strings"
	"testing"
)

// TestUpgradeNeverStampsTheBuildFromBeforeTheMerge.
//
// Make expands `:=` variables when it reads the Makefile, and VERSION is one of them. The upgrade
// recipe then fast forwards the checkout, so HEAD moves after that expansion. Any value the recipe
// expands for itself is the commit the checkout held before the merge, and the stack started with it
// reports a build one behind the tool and the sandbox image. The tool and the image escape it because
// they are built by sub makes, and a sub make reads this file again.
//
// So no line of the upgrade recipe after the merge may expand VERSION. The step that needs the stamp
// delegates to a target that runs as its own make.
func TestUpgradeNeverStampsTheBuildFromBeforeTheMerge(t *testing.T) {
	recipe := target(t, "upgrade")

	lines := strings.Split(recipe, "\n")
	merge := -1
	for i, line := range lines {
		if strings.Contains(line, "git merge") {
			merge = i
			break
		}
	}
	if merge < 0 {
		t.Fatalf("make upgrade no longer moves the checkout at all, so this test proves nothing:\n%s", recipe)
	}

	for _, line := range lines[merge+1:] {
		if strings.Contains(line, "$(VERSION)") {
			t.Errorf("this line runs after the merge moved HEAD and expands VERSION, so it carries the "+
				"commit from before the upgrade: %q", strings.TrimSpace(line))
		}
	}
}
