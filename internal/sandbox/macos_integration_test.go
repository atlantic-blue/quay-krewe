//go:build integration && darwin

package sandbox_test

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
	"github.com/atlantic-blue/quay-krewe/internal/sandbox/sandboxtest"
)

// The macOS backend against a real tart, which needs an Apple machine to run at all.
//
// Nothing in the pipeline runs this: every runner the project has is Linux. So it is written for the
// machine that has one, and it says out loud what it needs rather than passing quietly: a skip is not
// a pass, and the log names what was missing.
//
// QC_TEST_MACOS_IMAGE names the guest to clone, for example
// ghcr.io/cirruslabs/macos-tahoe-xcode:latest. It is tens of gigabytes, so it is asked for by
// configuration rather than pulled by a test nobody expected to pull it.

// needsTart says what is missing, and skips only for a reason it can name.
func needsTart(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("tart"); err != nil {
		t.Skip("tart is not on the path, so the macOS backend is unproved on this machine")
	}
	image := os.Getenv("QC_TEST_MACOS_IMAGE")
	if image == "" {
		t.Skip("set QC_TEST_MACOS_IMAGE to the guest to clone, and the macOS backend is unproved without it")
	}
	return image
}

// TestMacOSProviderConformanceAgainstRealTart is the same contract the container backend keeps,
// asked of a real macOS guest.
func TestMacOSProviderConformanceAgainstRealTart(t *testing.T) {
	image := needsTart(t)
	sandboxtest.RunConformance(t, func(*testing.T) sandbox.Provider {
		return &sandbox.MacOSProvider{Image: image}
	})
}

// TestASessionCanBuildADarwinApplication is why this backend exists. A Linux container runs the
// linter, the types and the tests of an iOS application and never builds it, so a native build is
// proved by hand on one machine after the work merged.
func TestASessionCanBuildADarwinApplication(t *testing.T) {
	image := needsTart(t)
	ctx, done := context.WithTimeout(context.Background(), 30*time.Minute)
	defer done()

	provider := &sandbox.MacOSProvider{Image: image}
	session := "0123456789abcdef0123456d"
	box, err := provider.Create(ctx, sandbox.Config{ID: session})
	if err != nil {
		t.Fatalf("create a guest: %v", err)
	}
	t.Cleanup(func() { _ = provider.Remove(context.Background(), session) })

	// A workspace of its own, built in the guest, so the assertion is about the toolchain rather than
	// about a repository the machine happens to hold.
	const project = `
set -e
mkdir -p /tmp/conformance/Sources/hello
cd /tmp/conformance
printf 'print("built in a guest")\n' > Sources/hello/main.swift
printf '// swift-tools-version:5.9\nimport PackageDescription\nlet package = Package(name: "hello", targets: [.executableTarget(name: "hello")])\n' > Package.swift
xcodebuild -scheme hello -destination 'platform=macOS' build
`
	proc, err := box.Exec(ctx, sandbox.Spec{Argv: []string{"sh", "-c", project}})
	if err != nil {
		t.Fatalf("run the build: %v", err)
	}
	said, err := io.ReadAll(proc.Stdout())
	if err != nil {
		t.Fatalf("read the build output: %v", err)
	}
	if err := proc.Wait(); err != nil {
		t.Fatalf("the guest could not build a darwin application: %v\n%s\n%s", err, said, proc.Stderr())
	}
	if !strings.Contains(string(said), "BUILD SUCCEEDED") {
		t.Fatalf("xcodebuild said:\n%s\nwant a build that succeeded", said)
	}
}

// TestTartHoldsTheLicensedNumberOfGuests asks the machine what the licence says, rather than trusting
// this project's reading of it. A host that permits a different number is an operator's own
// agreement with Apple, and the backend is told the number rather than guessing.
func TestTartHoldsTheLicensedNumberOfGuests(t *testing.T) {
	image := needsTart(t)
	ctx, done := context.WithTimeout(context.Background(), 30*time.Minute)
	defer done()

	// One more than the licence permits, taken past the pool so the framework itself answers.
	provider := &sandbox.MacOSProvider{Image: image, Guests: sandbox.LicensedGuests + 1}
	var made []string
	t.Cleanup(func() {
		for _, session := range made {
			_ = provider.Remove(context.Background(), session)
		}
	})

	sessions := []string{"0123456789abcdef0123451a", "0123456789abcdef0123451b", "0123456789abcdef0123451c"}
	var refusal error
	for _, session := range sessions {
		if _, err := provider.Create(ctx, sandbox.Config{ID: session}); err != nil {
			refusal = err
			break
		}
		made = append(made, session)
	}
	if len(made) > sandbox.LicensedGuests {
		t.Fatalf("this host ran %d macOS guests at once, and the licence this backend reads permits %d",
			len(made), sandbox.LicensedGuests)
	}
	if refusal == nil {
		t.Fatalf("the runtime accepted %d guests without refusing one", len(made))
	}
	t.Logf("the runtime refused guest %d with: %v", len(made)+1, refusal)
}
