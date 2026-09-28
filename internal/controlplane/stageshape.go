package controlplane

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/atlantic-blue/quay-krewe/internal/contextsize"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The shape of one design stage: seven headings, in one order, inside one page.
//
// A stage is the only thing the operator reads about a part of the design, and they read it alone
// and decide on it alone. Nothing held it to a shape before, so a stage was whatever a session
// wrote, and what a session writes is its research: the files it opened, the lines it read, the
// counts it made, a section for what it read and a section for what it did not. None of that is a
// decision, none of it survives the next commit, and an operator handed fifty thousand characters of
// it reads none of it.
//
// So the seven headings are the refusal, not a suggestion. Each one is a thing the operator can
// agree with or change: what this is for, what we recommend, what we assumed, what they must decide,
// how we will know it is done, what we are leaving out, and what we could not safely guess. The
// length is the other half: a document that will not fit on a page is a document still carrying the
// reading it was written from.

// StageCeiling is the longest a stage body may be, in characters.
//
// It is a refusal rather than a warning, which is the whole change. A stage used to be measured
// against the mark the design body is measured against, which warns and keeps the text, and that is
// how a stage of 51,176 characters reached an operator who then stopped reading the stages at all.
//
// The number is measured rather than picked. The discovery the discover skill ships is about 2,600
// characters, and the fullest stage of the six is the design system, which carries a line for each
// decision: twenty one of those, with a direction and the other five headings around them, comes to
// about 4,700. Six thousand leaves that room and refuses everything that is a repository pasted in.
const StageCeiling = 6_000

// StageHeadings are the seven headings of a stage, in the order a reader meets them.
//
// The order carries meaning. The direction is the recommendation, so it sits under the goal it
// serves and above the decisions it asks for. What we are not doing sits under what done looks like,
// because it is read as the edge of it.
func StageHeadings() []string {
	return []string{
		"Goal",
		"Direction",
		"Assumptions",
		"Decisions for the operator",
		"Done when",
		"Not doing",
		"Open questions",
	}
}

// aHeading matches one markdown heading line, at any level, and captures its text.
//
// Any level, because a stage written with `# Goal` and one written with `## Goal` say the same thing
// to a reader and refusing one of them teaches nothing. A trailing colon is allowed for the same
// reason. The text is captured rather than matched against a name, so the order of the headings a
// body actually holds can be read off in one pass.
var aHeading = regexp.MustCompile(`(?m)^[ \t]*#{1,6}[ \t]+([^\n#][^\n]*?)[ \t]*:?[ \t]*\r?$`)

// checkStageShape refuses a body that is not a brief: one that leaves a heading out, one that puts
// them in another order, and one that runs past a page.
//
// It runs on all six stages. The diagram rule binds two of them because only two describe a
// structure; this one binds every stage, because every stage is a thing the operator has to decide
// on, and a decision buried in a research dump is a decision nobody makes.
//
// The headings are read before the length, because a body that is over the ceiling and also missing
// its headings is a body to rewrite rather than to cut, and the heading refusal is the one that says
// so. It runs before the store is asked to write, so a refused stage leaves nothing behind.
func checkStageShape(stage, body string) error {
	if err := checkStageHeadings(stage, body); err != nil {
		return err
	}
	if length := utf8.RuneCountInString(body); length > StageCeiling {
		return status.Errorf(codes.InvalidArgument,
			"the %s stage is %s, and a stage is one page: write at most %s, and take out the "+
				"reading it was written from. The findings belong in the stage; the file paths, the "+
				"line numbers and the counts belong in the code the next session reads",
			stage, contextsize.Characters(length), contextsize.Characters(StageCeiling))
	}
	return nil
}

// checkStageHeadings refuses a body missing one of the seven, and a body holding all seven in
// another order.
//
// A missing heading is named one at a time. An operator told only that the shape is wrong has to
// work out which of seven they left out, and the session that wrote it has to guess the same.
func checkStageHeadings(stage, body string) error {
	at := map[string]int{}
	for _, found := range aHeading.FindAllStringSubmatchIndex(body, -1) {
		text := strings.TrimSpace(body[found[2]:found[3]])
		for _, heading := range StageHeadings() {
			if strings.EqualFold(text, heading) {
				if _, already := at[heading]; !already {
					at[heading] = found[0]
				}
			}
		}
	}

	for _, heading := range StageHeadings() {
		if _, held := at[heading]; !held {
			return status.Errorf(codes.InvalidArgument,
				"the %s stage carries no %q heading: a stage is a brief and it holds these seven, "+
					"in this order, each written as `## <heading>`: %s",
				stage, heading, strings.Join(StageHeadings(), ", "))
		}
	}

	held := StageHeadings()
	for i := 1; i < len(held); i++ {
		if at[held[i]] < at[held[i-1]] {
			return status.Errorf(codes.InvalidArgument,
				"the %s stage puts %q above %q, and the seven headings are read in one order: %s",
				stage, held[i], held[i-1], strings.Join(held, ", "))
		}
	}
	return nil
}
