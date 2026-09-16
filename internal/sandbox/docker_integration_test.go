//go:build integration

package sandbox_test

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/atlantic-blue/quay-krewe/internal/sandbox"
	"github.com/atlantic-blue/quay-krewe/internal/skill"
)

// What is left in here is about the sandbox image rather than about the Docker backend: what the
// image carries, and what a session can do inside it. The backend contract itself is in
// internal/sandbox/sandboxtest, where Docker and Apple container run the same cases.

// TestASessionCanActuallyCommit is the whole point of an identity, and the only way to know it is
// right is to make a commit in a real container and read the author back.
//
// The environment is what carries it: git reads GIT_AUTHOR_NAME, GIT_AUTHOR_EMAIL and the committer
// pair beside them, and refuses when any is missing rather than guessing. A test that asserted the
// four variables were set would have passed just as happily with the wrong names.
func TestASessionCanActuallyCommit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	image := os.Getenv("QC_TEST_SANDBOX_IMAGE")
	if image == "" {
		t.Skip("set QC_TEST_SANDBOX_IMAGE to an image with git in it")
	}

	provider := sandbox.DockerProvider{Image: image}
	box, err := provider.Create(ctx, sandbox.Config{
		ID: "gitidentity" + strings.Repeat("0", 13),
		Env: []string{
			"GIT_AUTHOR_NAME=A Name", "GIT_AUTHOR_EMAIL=a@example.com",
			"GIT_COMMITTER_NAME=A Name", "GIT_COMMITTER_EMAIL=a@example.com",
		},
	})
	if err != nil {
		t.Fatalf("create the sandbox: %v", err)
	}
	defer func() { _ = box.Close(ctx) }()

	proc, err := box.Exec(ctx, sandbox.Spec{Argv: []string{"sh", "-c",
		"cd /tmp && git init -q repo && cd repo && echo hi > a.txt && git add a.txt && " +
			"git commit -q -m probe && git log --format='%an <%ae>|%cn <%ce>' -1"}})
	if err != nil {
		t.Fatalf("run git in the sandbox: %v", err)
	}
	said, err := io.ReadAll(proc.Stdout())
	if err != nil {
		t.Fatalf("read what git said: %v", err)
	}
	if err := proc.Wait(); err != nil {
		t.Fatalf("git refused to commit: %v: %s", err, proc.Stderr())
	}

	// The author and the committer, both of them, because git names them separately and a session
	// commits as the operator rather than on behalf of somebody else.
	if got := strings.TrimSpace(string(said)); got != "A Name <a@example.com>|A Name <a@example.com>" {
		t.Fatalf("the commit is by %q", got)
	}
}

// A session clones in conversation, so the image has to answer git's credential query itself: the
// helper registered in the system configuration reads GH_TOKEN at the moment git asks. Asked of git
// inside a real container rather than of the files, because a helper that is shipped but never
// registered, or registered under a path nothing ships, passes every read of the Dockerfile.
func TestGitFindsItsCredentialWithoutArguments(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	image := os.Getenv("QC_TEST_SANDBOX_IMAGE")
	if image == "" {
		t.Skip("set QC_TEST_SANDBOX_IMAGE to an image with git in it")
	}

	provider := sandbox.DockerProvider{Image: image}
	box, err := provider.Create(ctx, sandbox.Config{
		ID:  "gitcredential" + strings.Repeat("0", 11),
		Env: []string{"GH_TOKEN=a-token-for-this-test"},
	})
	if err != nil {
		t.Fatalf("create the sandbox: %v", err)
	}
	defer func() { _ = box.Close(ctx) }()

	proc, err := box.Exec(ctx, sandbox.Spec{Argv: []string{"sh", "-c",
		"printf 'protocol=https\nhost=github.com\n\n' | git credential fill"}})
	if err != nil {
		t.Fatalf("ask git for a credential: %v", err)
	}
	said, err := io.ReadAll(proc.Stdout())
	if err != nil {
		t.Fatalf("read what git said: %v", err)
	}
	if err := proc.Wait(); err != nil {
		t.Fatalf("git could not fill the credential: %v: %s", err, proc.Stderr())
	}

	answer := string(said)
	if !strings.Contains(answer, "username=x-access-token") {
		t.Errorf("git answered %q, want the helper's username", answer)
	}
	if !strings.Contains(answer, "password=a-token-for-this-test") {
		t.Errorf("git answered without the token from GH_TOKEN, so a private clone in conversation has nothing to authenticate with")
	}
}

// Every binary a shipped skill declares has to actually be in the image, or the declaration is a
// promise the sandbox breaks on somebody's first exec. One guard over the whole class, so the next
// skill cannot repeat the gap: the set of binaries is read from skills/ itself, and each is asked
// for inside a real container rather than looked for in the Dockerfile.
func TestTheImageCarriesEveryBinaryAShippedSkillDeclares(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	image := os.Getenv("QC_TEST_SANDBOX_IMAGE")
	if image == "" {
		t.Skip("set QC_TEST_SANDBOX_IMAGE to the sandbox image")
	}

	shipped, err := skill.Load("../../skills")
	if err != nil {
		t.Fatalf("loading the shipped skills: %v", err)
	}
	binaries := map[string]bool{}
	for _, one := range shipped {
		for _, binary := range one.Binaries {
			binaries[binary] = true
		}
	}
	if len(binaries) == 0 {
		t.Fatal("no shipped skill declares any binary, so this test proves nothing")
	}

	provider := sandbox.DockerProvider{Image: image}
	box, err := provider.Create(ctx, sandbox.Config{
		ID: "skillbinaries" + strings.Repeat("0", 11),
	})
	if err != nil {
		t.Fatalf("create the sandbox: %v", err)
	}
	defer func() { _ = box.Close(ctx) }()

	for binary := range binaries {
		proc, err := box.Exec(ctx, sandbox.Spec{Argv: []string{"sh", "-c", `command -v -- "$1"`, "sh", binary}})
		if err != nil {
			t.Fatalf("ask for %s in the sandbox: %v", binary, err)
		}
		_, _ = io.Copy(io.Discard, proc.Stdout())
		if err := proc.Wait(); err != nil {
			t.Errorf("a shipped skill declares %s and the image does not carry it: %v: %s", binary, err, proc.Stderr())
		}
	}
}

// A mounted secret has to be a file the session's own user can read, on a mount that is memory
// backed, and readable by nobody else. Every one of those is a property of the daemon rather than of
// the code, so this asks the daemon.
//
// It ran against the real sandbox image on purpose. A looser image manufactures green here: busybox
// runs as root, so a write into a directory owned by root succeeds there and fails in the image
// sessions actually use, where the user is not root and the mount would have belonged to root.
func TestAMountedSecretIsAFileTheSandboxUserCanReadAndNobodyElseCan(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	image := os.Getenv("QC_TEST_SANDBOX_IMAGE")
	if image == "" {
		t.Skip("set QC_TEST_SANDBOX_IMAGE to the sandbox image")
	}

	provider := sandbox.DockerProvider{Image: image}
	box, err := provider.Create(ctx, sandbox.Config{ID: "secretfile" + strings.Repeat("0", 13)})
	if err != nil {
		t.Fatalf("create the sandbox: %v", err)
	}
	defer func() { _ = box.Close(ctx) }()

	// The same shape the system uses: the value through the environment of the one command, never as an
	// argument, and umask before the write so the file is never briefly readable.
	const contents = "[user]\n\tname = operator\n"
	at := sandbox.SecretFilePath("gitconfig")
	write, err := box.Exec(ctx, sandbox.Spec{
		Argv: []string{"sh", "-c", "set -e\numask 077\nmkdir -p " + sandbox.SecretsPath +
			"\nprintf '%s' \"$QC_SECRET_FILE_VALUE\" > " + at},
		Env: []string{"QC_SECRET_FILE_VALUE=" + contents},
	})
	if err != nil {
		t.Fatalf("write the secret: %v", err)
	}
	_, _ = io.ReadAll(write.Stdout())
	if err := write.Wait(); err != nil {
		t.Fatalf("the sandbox user could not write into %s: %v: %s", sandbox.SecretsPath, err, write.Stderr())
	}

	read, err := box.Exec(ctx, sandbox.Spec{Argv: []string{"sh", "-c",
		"cat " + at + " && echo '|' && stat -c '%a %U' " + at +
			" && echo '|' && stat -f -c %T " + sandbox.SecretsPath}})
	if err != nil {
		t.Fatalf("read the secret back: %v", err)
	}
	said, err := io.ReadAll(read.Stdout())
	if err != nil {
		t.Fatalf("read what the sandbox said: %v", err)
	}
	if err := read.Wait(); err != nil {
		t.Fatalf("the sandbox user could not read the file it was given: %v: %s", err, read.Stderr())
	}

	parts := strings.Split(string(said), "|\n")
	if len(parts) != 3 {
		t.Fatalf("the sandbox said %q, which is not the three answers asked for", said)
	}
	if parts[0] != contents {
		t.Fatalf("the file holds %q, want the bytes it was given", parts[0])
	}
	// Readable and writable by the sandbox user, and nothing to anybody else. The umask is what makes
	// this 600, and a file that arrives readable is one any other process in the container can take.
	if got := strings.TrimSpace(parts[1]); got != "600 agent" {
		t.Fatalf("the file is %q, want it owned by the sandbox user and shut to everybody else", got)
	}
	// Memory backed, so the value never reaches the container's writable layer or the host's disk.
	// The whole reason a mounted credential is safer than one in the environment rests on this.
	if got := strings.TrimSpace(parts[2]); got != "tmpfs" {
		t.Fatalf("%s is a %s, want tmpfs", sandbox.SecretsPath, got)
	}
}
