package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// marked is a session directory the system has put under the gate, and unmarked is one it has not.
func marked(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, MarkDir), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, MarkDir, MarkFile), []byte("building"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return dir
}

// What the runtime sends, and what it reads back. The exit code is the whole answer: 2 stops the tool
// call, anything else lets it run, so a hook that returns the wrong number on a payload it did not
// expect either stops the session working or stops guarding.
func TestTheHookAnswersWhatTheRuntimeSends(t *testing.T) {
	payloads := []struct {
		name string
		body string
		// set is the environment variable, which the system no longer writes and this build still
		// reads. mark is the file in the session's own directory, which is what turns the gate on now.
		set  bool
		mark bool
		want int
	}{
		{
			name: "a write to a test is refused",
			body: `{"tool_name":"Write","tool_input":{"file_path":"internal/job/build_test.go"}}`,
			mark: true,
			want: Refused,
		},
		{
			name: "a write to the code is not",
			body: `{"tool_name":"Write","tool_input":{"file_path":"internal/job/build.go"}}`,
			mark: true,
			want: 0,
		},
		{
			name: "reading a test is not",
			body: `{"tool_name":"Bash","tool_input":{"command":"cat internal/job/build_test.go"}}`,
			mark: true,
			want: 0,
		},
		{
			name: "a shell that writes a test is refused",
			body: `{"tool_name":"Bash","tool_input":{"command":"echo x > internal/job/build_test.go"}}`,
			mark: true,
			want: Refused,
		},
		{
			name: "taking the mark away is refused",
			body: `{"tool_name":"Bash","tool_input":{"command":"rm .krewe/building"}}`,
			mark: true,
			want: Refused,
		},
		{
			name: "a session that is not building is left alone",
			body: `{"tool_name":"Write","tool_input":{"file_path":"internal/job/build_test.go"}}`,
			want: 0,
		},
		{
			name: "the variable still says a session is building",
			body: `{"tool_name":"Write","tool_input":{"file_path":"internal/job/build_test.go"}}`,
			set:  true,
			want: Refused,
		},
		// The three below decide whether a broken hook can stop a system. It fires on every write every
		// session makes, so anything it cannot read has to be let through.
		{name: "a payload that is not json", body: "not json at all", mark: true, want: 0},
		{name: "a payload with no tool", body: `{"hook_event_name":"PreToolUse"}`, mark: true, want: 0},
		{name: "nothing at all", body: "", mark: true, want: 0},
	}
	for _, payload := range payloads {
		t.Run(payload.name, func(t *testing.T) {
			here := t.TempDir()
			if payload.mark {
				here = marked(t)
			}
			said := &strings.Builder{}
			if code := Run(strings.NewReader(payload.body), said, payload.set, here); code != payload.want {
				t.Fatalf("the hook answered %d, want %d, saying %q", code, payload.want, said)
			}
			// A refusal the session cannot read is a tool call that fails for no stated reason.
			if payload.want == Refused && said.Len() == 0 {
				t.Fatal("the hook refused and told the session nothing")
			}
			if payload.want == 0 && said.Len() != 0 {
				t.Fatalf("the hook allowed the call and said %q", said)
			}
		})
	}
}

// Where the session is working is the runtime's word first and this process's directory second. A
// hook started somewhere else would read the wrong directory for the mark and answer that a session
// under the gate is free.
func TestTheHookReadsTheMarkWhereTheRuntimeSaysTheSessionIs(t *testing.T) {
	body := `{"tool_name":"Write","cwd":"%s","tool_input":{"file_path":"internal/job/build_test.go"}}`
	said := &strings.Builder{}

	payload := strings.Replace(body, "%s", marked(t), 1)
	if code := Run(strings.NewReader(payload), said, false, t.TempDir()); code != Refused {
		t.Fatalf("the hook answered %d for a session the runtime says is marked, want %d", code, Refused)
	}

	said.Reset()
	payload = strings.Replace(body, "%s", t.TempDir(), 1)
	if code := Run(strings.NewReader(payload), said, false, marked(t)); code != 0 {
		t.Fatalf("the hook answered %d for a session the runtime says is elsewhere, want 0: %s", code, said)
	}
}
