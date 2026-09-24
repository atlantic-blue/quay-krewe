package proofcommand_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/atlantic-blue/quay-krewe/internal/proofcommand"
)

// The names a shell reads as code. Each one is a scenario name somebody would write, and the character
// that makes it dangerous is in it: the quote that ends a quoted string, the backtick and the dollar
// sign that run a command, the backslash that escapes, the space that splits one word into two.
var theNames = []struct {
	name string
	is   string
}{
	{name: "a single quote", is: "A check with no checkout in the session's working tree is refused"},
	{name: "a double quote", is: "A name with \"quotes\" around two words"},
	{name: "a backtick", is: "A key of `../../etc/passwd` resolves to a name inside the directory"},
	{name: "a dollar sign", is: "A run that costs $0 and reads $HOME"},
	{name: "a backslash", is: "A name with a \\ in it"},
	{name: "a space", is: "two words"},
	{name: "no name at all", is: ""},
	{name: "all of them at once", is: "A key of `../../etc/passwd` isn't \"outside\" it, and costs $0"},
}

// A real shell, because the danger is the shell and a test that read the string back would only be
// asking the code to agree with itself.
func TestAShellReadsTheWordBackAsTheName(t *testing.T) {
	for _, held := range theNames {
		t.Run(held.name, func(t *testing.T) {
			out, err := exec.Command("sh", "-c", "printf %s "+proofcommand.ShellWord(held.is)).Output()
			if err != nil {
				t.Fatalf("the shell refused the word %s: %v", proofcommand.ShellWord(held.is), err)
			}
			if string(out) != held.is {
				t.Fatalf("the shell read %q, want %q", out, held.is)
			}
		})
	}
}

// The name has to arrive as one argument as well as unchanged. A format around each argument says which
// it was: one word prints inside one pair of brackets, and a name the shell split prints in several.
func TestTheNameArrivesAsOneWord(t *testing.T) {
	for _, held := range theNames {
		t.Run(held.name, func(t *testing.T) {
			out, err := exec.Command("sh", "-c", "printf [%s] "+proofcommand.ShellWord(held.is)).Output()
			if err != nil {
				t.Fatalf("the shell refused the word %s: %v", proofcommand.ShellWord(held.is), err)
			}
			if want := "[" + held.is + "]"; string(out) != want {
				t.Fatalf("the shell read %q, want %q, so the name did not arrive as one word", out, want)
			}
		})
	}
}

// What the three call sites do, in one function, so the run, the preview and the printout cannot drift
// apart on the quoting.
func TestSubstitutePutsTheNameWhereTheTokenWas(t *testing.T) {
	for _, held := range theNames {
		t.Run(held.name, func(t *testing.T) {
			command := proofcommand.Substitute("printf [%s] "+proofcommand.Token, held.is)
			out, err := exec.Command("sh", "-c", command).Output()
			if err != nil {
				t.Fatalf("the shell refused %q: %v", command, err)
			}
			if want := "[" + held.is + "]"; string(out) != want {
				t.Fatalf("the shell read %q, want %q", out, want)
			}
		})
	}
}

// Krewe supplies the quotes, so a command that supplies its own puts the name inside a second pair and
// splits it again. The scanner reads the shell's own rules: a backslash escapes outside quotes and
// inside double quotes, and nothing escapes inside single quotes.
func TestATokenInsideQuotesIsFound(t *testing.T) {
	for _, test := range []struct {
		name    string
		command string
		inside  bool
	}{
		{
			name:    "inside single quotes",
			command: "go test ./features/... -run 'TestFeatures/{scenario}'",
			inside:  true,
		},
		{
			name:    "inside double quotes",
			command: "go test ./features/ -run \"TestFeatures/{scenario}$\"",
			inside:  true,
		},
		{
			name:    "outside, with a quoted dollar sign after it",
			command: "go test ./features/ -count=1 -v -run TestFeatures/{scenario}'$'",
			inside:  false,
		},
		{
			name:    "outside, and nothing is quoted at all",
			command: "make one {scenario}",
			inside:  false,
		},
		{
			name:    "outside, after a span that closed",
			command: "sh -c 'cd features' && go test -run {scenario}",
			inside:  false,
		},
		{
			name:    "outside, after a quote the shell does not read",
			command: "go test -run \\'{scenario}",
			inside:  false,
		},
		{
			name:    "outside, because a backslash does not escape inside single quotes",
			command: "go test -run 'TestFeatures\\'{scenario}'",
			inside:  false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := proofcommand.TokenInsideQuotes(test.command); got != test.inside {
				t.Fatalf("%q reads the token as inside quotes: %t, want %t", test.command, got, test.inside)
			}
		})
	}
}

// The refusal shows the command to type rather than only saying no, because a person who reads it is
// about to type the command again. What was quoted around the token stays quoted where the shell reads
// it, and loses the quotes where it does not.
func TestTheAdviceMovesTheTokenOutsideTheQuotes(t *testing.T) {
	for _, test := range []struct {
		name    string
		command string
		advice  string
	}{
		{
			name:    "single quotes around the token alone",
			command: "go test ./features/... -run 'TestFeatures/{scenario}'",
			advice:  "go test ./features/... -run TestFeatures/{scenario}",
		},
		{
			name:    "double quotes carrying a dollar sign the shell reads",
			command: "go test ./features/ -count=1 -v -run \"TestFeatures/{scenario}$\"",
			advice:  "go test ./features/ -count=1 -v -run TestFeatures/{scenario}'$'",
		},
		{
			name:    "a command that already writes it this way is left alone",
			command: "go test ./features/ -run TestFeatures/{scenario}'$'",
			advice:  "go test ./features/ -run TestFeatures/{scenario}'$'",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := proofcommand.TokenOutsideQuotes(test.command)
			if got != test.advice {
				t.Fatalf("the advice for %q is %q, want %q", test.command, got, test.advice)
			}
			if proofcommand.TokenInsideQuotes(got) {
				t.Fatalf("the advice %q still carries the token inside quotes", got)
			}
		})
	}
}

// The advice is a command a shell can read, and the name still arrives as one word through it.
func TestTheAdviceIsACommandAShellCanRun(t *testing.T) {
	advice := proofcommand.TokenOutsideQuotes(`printf [%s] '` + proofcommand.Token + `'`)
	if strings.Contains(advice, `'`+proofcommand.Token+`'`) {
		t.Fatalf("the advice left the token inside the quotes: %q", advice)
	}
	out, err := exec.Command("sh", "-c", proofcommand.Substitute(advice, "two words")).Output()
	if err != nil {
		t.Fatalf("the shell refused %q: %v", advice, err)
	}
	if want := "[two words]"; string(out) != want {
		t.Fatalf("the shell read %q, want %q", out, want)
	}
}
