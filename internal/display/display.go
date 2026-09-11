// Package display renders identifiers and names for a human reading a list, so the console and the
// command line tool shorten and label things the same way.
package display

import "fmt"

// shortIDLength is eight hex characters of a twelve byte identifier: four billion values, and a row
// that reads as a row rather than a wall of hex. Actions use the full value.
const shortIDLength = 8

// ShortID is safe to apply to values that are not identifiers: anything short enough is left alone.
func ShortID(id string) string {
	if len(id) <= shortIDLength {
		return id
	}
	return id[:shortIDLength]
}

// LooksLikeIdentifier says whether a word is shaped like the identifier a listing prints: hexadecimal,
// and at least as long as the column is wide.
//
// It decides whether one word of a command line is a session or the first word of a message, so it
// has to be narrow. An ordinary English word is not hexadecimal, and the few that are are shorter
// than eight letters.
func LooksLikeIdentifier(word string) bool {
	if len(word) < shortIDLength {
		return false
	}
	for _, character := range word {
		switch {
		case character >= '0' && character <= '9':
		case character >= 'a' && character <= 'f':
		case character >= 'A' && character <= 'F':
		default:
			return false
		}
	}
	return true
}

// Name prefers the name and falls back to the identifier, for example a session whose workspace has
// been deleted. Never blank: a blank cell reads as a bug.
func Name(name, id string) string {
	if name != "" {
		return name
	}
	if id == "" {
		return "-"
	}
	return ShortID(id)
}

// Scenarios is how many scenarios a run reported, as a line says it, in the singular where it
// reported one.
//
// It is here rather than in either caller because the path document a session reads and the verdict
// the command line prints say the same thing, and a reader who finds "1 scenarios" under a step reads
// the whole line as generated rather than as a record.
func Scenarios(count int32) string {
	if count == 1 {
		return "1 scenario"
	}
	return fmt.Sprintf("%d scenarios", count)
}
