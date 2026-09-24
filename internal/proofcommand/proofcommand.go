// Package proofcommand composes the command krewe runs to prove one step of a path.
//
// A proof command carries a token where the scenario name goes, and krewe runs the finished command
// with a shell. A scenario name is prose: it holds spaces, quotes, backticks and dollar signs, and a
// shell reads every one of them as code. So the name is quoted here, in one place the control plane and
// the command line tool both read, because a second copy of this would be a second answer to what the
// shell is about to be handed.
package proofcommand

import "strings"

// Token is what a proof command carries where the name of one scenario goes.
const Token = "{scenario}"

// ShellWord is the name as one shell word.
//
// Single quotes, because they are the only quoting a shell reads nothing inside: no expansion, no
// command substitution, no escape. The name can then hold any character at all. A single quote in the
// name is the one thing that cannot sit in there, so it closes the word, adds an escaped quote, and
// opens the next one.
func ShellWord(name string) string {
	return "'" + strings.ReplaceAll(name, "'", `'\''`) + "'"
}

// Substitute is the command with the scenario name as one word where the token was.
func Substitute(command, name string) string {
	return strings.ReplaceAll(command, Token, ShellWord(name))
}

// TokenInsideQuotes says whether the token sits inside a quoted span of the command.
//
// Krewe supplies the quotes, so a command that supplies its own puts the name inside a second pair. The
// name then closes that pair in the middle of itself and the rest of it splits into words.
func TokenInsideQuotes(command string) bool {
	return theQuotedToken(command) >= 0
}

// theQuotedToken is where the first token the shell reads inside quotes starts, and a position nothing
// should read where every token of the command sits outside them. Every occurrence is read rather than
// the first, because the run substitutes every one of them.
func theQuotedToken(command string) int {
	for from := 0; from <= len(command)-len(Token); {
		found := strings.Index(command[from:], Token)
		if found < 0 {
			return -1
		}
		found += from
		if span, _, _ := theSpanAround(command, found); span != bare {
			return found
		}
		from = found + len(Token)
	}
	return -1
}

// TokenOutsideQuotes is the same command written with the token outside the quotes.
//
// The refusal shows this rather than only saying no, because a person who reads it is about to type the
// command again. What sat in the quotes beside the token keeps quotes of its own where the shell reads
// it, and loses them where it does not, so the command still means what it meant.
func TokenOutsideQuotes(command string) string {
	for written := command; ; {
		at := theQuotedToken(written)
		if at < 0 {
			return written
		}
		_, opened, closed := theSpanAround(written, at)
		inside := written[opened+1 : closed]
		held := at - (opened + 1)
		after := written[min(closed+1, len(written)):]
		written = written[:opened] + quotedWhereItHasTo(inside[:held]) + Token +
			quotedWhereItHasTo(inside[held+len(Token):]) + after
	}
}

// quoting is what the shell reads a part of a command as.
type quoting int

const (
	bare quoting = iota
	single
	double
)

// theSpanAround is the quoting the shell is in at that position, and where the span it is in opens and
// closes. A position the shell reads bare answers bare and two positions nothing should read.
//
// It reads the shell's own rules. Outside quotes a backslash escapes the character after it. Inside
// single quotes nothing escapes, so the next single quote ends the span however many backslashes are in
// front of it. Inside double quotes a backslash escapes only the four characters that mean something in
// there. A span nothing closes is read as closing at the end of the command, because that is the part
// of the command the token is in.
func theSpanAround(command string, at int) (quoting, int, int) {
	span, opened := bare, -1
	for i := 0; i < len(command); i++ {
		if i == at {
			if span == bare {
				return bare, -1, -1
			}
			return span, opened, theEndOfTheSpan(command, span, i)
		}
		switch character := command[i]; span {
		case bare:
			switch character {
			case '\\':
				i++
			case '\'':
				span, opened = single, i
			case '"':
				span, opened = double, i
			}
		case single:
			if character == '\'' {
				span, opened = bare, -1
			}
		case double:
			switch {
			case character == '\\' && i+1 < len(command) && escapedInDoubleQuotes(command[i+1]):
				i++
			case character == '"':
				span, opened = bare, -1
			}
		}
	}
	return bare, -1, -1
}

// theEndOfTheSpan is where the span the token sits in closes, or the end of the command where nothing
// closes it.
func theEndOfTheSpan(command string, span quoting, from int) int {
	for i := from; i < len(command); i++ {
		character := command[i]
		if span == single {
			if character == '\'' {
				return i
			}
			continue
		}
		switch {
		case character == '\\' && i+1 < len(command) && escapedInDoubleQuotes(command[i+1]):
			i++
		case character == '"':
			return i
		}
	}
	return len(command)
}

// escapedInDoubleQuotes are the characters a backslash escapes inside double quotes. A backslash in
// front of anything else is a backslash.
func escapedInDoubleQuotes(character byte) bool {
	return character == '"' || character == '\\' || character == '$' || character == '`'
}

// quotedWhereItHasTo is that part of a command as the shell reads it now: bare where every character in
// it means itself, and quoted where one of them does not.
func quotedWhereItHasTo(part string) string {
	if part == "" || readAsItself(part) {
		return part
	}
	return ShellWord(part)
}

// readAsItself says whether a shell reads every character of this part as the character it is. The set
// is deliberately short: a character nobody is sure about is quoted, which is always safe and only
// costs two characters of a message.
func readAsItself(part string) bool {
	for _, character := range part {
		switch {
		case character >= 'a' && character <= 'z':
		case character >= 'A' && character <= 'Z':
		case character >= '0' && character <= '9':
		case strings.ContainsRune("._/-:=,+@", character):
		default:
			return false
		}
	}
	return true
}
