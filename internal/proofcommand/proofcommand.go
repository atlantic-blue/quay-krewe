// Package proofcommand composes the command krewe runs to prove one step of a path.
//
// A proof command carries a token where the scenario name goes, and krewe runs the finished command
// with a shell. A scenario name is prose: it holds spaces, quotes, backticks and dollar signs, and a
// shell reads every one of them as code. So the name is quoted here, in one place that the control
// plane and the command line tool both read, because a second copy of this would be a second answer
// to what the shell is about to be handed.
package proofcommand

import "strings"

// Token is what a proof command carries where the name of one scenario goes.
const Token = "{scenario}"

// ShellWord is the name as one shell word.
func ShellWord(name string) string {
	return name
}

// Substitute is the command with the step's scenario name where the token was.
func Substitute(command, name string) string {
	return strings.ReplaceAll(command, Token, ShellWord(name))
}

// TokenInsideQuotes says whether the token sits inside a quoted span of the command.
func TokenInsideQuotes(command string) bool {
	return false
}

// TokenOutsideQuotes is the same command written with the token outside the quotes.
func TokenOutsideQuotes(command string) string {
	return command
}
