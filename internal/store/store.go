// Package store is the durable home of workspaces, their channels, and their sessions.
//
// It exists because the control plane must hold no state of its own. A session's handle to the model
// conversation lives here, so a restart resumes the conversation instead of orphaning it: the
// conversation still exists inside the model's own store, and the pointer to it is the only thing
// that can be lost.
//
// Two implementations, one behaviour. Memory is for tests and for running without a database;
// Postgres is what the composed stack and the cloud use. Both are held to the same conformance suite
// in internal/store/storetest, so a behaviour proven against one is proven against the other.
package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/deploy"
	"github.com/atlantic-blue/quay-krewe/internal/hook"
	"github.com/atlantic-blue/quay-krewe/internal/session"
	"github.com/atlantic-blue/quay-krewe/internal/skill"
	"github.com/google/uuid"
)

// A conversation is unbounded and a terminal is not, so asking for everything gets the most recent
// slice of it.
const (
	defaultExecLimit = 50
	maxExecLimit     = 500
)

// ExecLimit applies the default when nothing was asked for and the ceiling when too much was.
func ExecLimit(limit int) int {
	switch {
	case limit <= 0:
		return defaultExecLimit
	case limit > maxExecLimit:
		return maxExecLimit
	default:
		return limit
	}
}

// ErrNotFound covers deleted as well as never existed.
var ErrNotFound = errors.New("store: not found")

// ErrNothingToApprove is returned when a design with no body is approved.
//
// The check and the write are one statement, so the refusal lives here rather than above: a read
// then a write would let a design emptied between the two come back approved.
var ErrNothingToApprove = errors.New("store: there is no design to approve")

// ErrSessionHoldsAnExec is returned when a session that has an exec in flight is asked to be
// archived.
//
// The check and the write are one statement, for the reason ErrNothingToApprove is one: a read of the
// status followed by a write would let an exec start between the two, and the session would then be
// put away with an open exec in it. The operator would lose the answer, and the answer is the work.
var ErrSessionHoldsAnExec = errors.New("store: the session holds an exec that is still running")

// ErrStepNotReady is returned when a step somebody asks for is not in state ready.
//
// The check and the write are one statement, for the reason ErrNothingToApprove is one: a read of
// the state followed by a write would let two callers both take one step, and each would be told a
// session holds it that is not the session it started.
var ErrStepNotReady = errors.New("store: the step is not ready to be taken")

// ErrPathHoldsTakenSteps is returned when a path write drops or renames a step that is taken, done
// or stopped. A ProtectedStepsError carries the numbers and answers errors.Is for this.
//
// The read of the states and the write are one transaction, for the reason ErrNothingToApprove is
// one statement: a take that landed between a read and a write would be deleted by the write, and
// the record of the work would go with it.
var ErrPathHoldsTakenSteps = errors.New("store: the path drops or renames a step somebody took")

// ErrTooManyStepsInFlight is returned when a project already holds as many steps in state taken as
// its cap allows. A StepsInFlightError carries the steps and the cap, and answers errors.Is for this.
//
// The count and the write are one transaction, for the reason ErrNothingToApprove is one statement: a
// count read in the control plane, then a write, would let two takes at one moment both pass a cap
// that has room for one of them.
//
// The count reads the whole project rather than one feature. Two features are two paths and one
// machine, so a cap read inside a feature would let a project with five features run five times the
// number the operator set.
var ErrTooManyStepsInFlight = errors.New("store: the project already has as many steps in flight as its cap allows")

// ErrStepsTouchTheSameFile is returned when a line of the step being taken names a file a step in
// state taken already names. A SharedFileError carries the file and that step, and answers errors.Is
// for this.
//
// The read of the files and the write are one transaction, for the reason ErrTooManyStepsInFlight is
// one: two takes at one moment, each reading before the other wrote, would both pass and put two
// sessions on one file.
//
// The files are read across the whole project rather than inside one feature. Two features share the
// user model, the router and the configuration, so a check scoped to a feature would let two of them
// write one file at the same moment.
var ErrStepsTouchTheSameFile = errors.New("store: a step in flight already writes that file")

// ErrPredecessorNotDone is returned when the step named by after is not in state done. This is gate
// 2. A PredecessorError carries that step and its state, and answers errors.Is for this.
//
// It reads the state and never the proof state. Done is the operator's word, so a step whose check
// failed and which the operator then closed opens this gate: the row records the disagreement and
// the path carries on. A gate reading a passing proof would hold the path on krewe's verdict and
// take the decision off the person whose decision it is.
//
// The check and the write are one statement, for the reason ErrNothingToApprove is one: a read of
// the predecessor followed by a write would let the step it names be stopped between the two, and
// the take would land on a path that moved.
//
// It stays inside the feature. A step waits for a lower step of its own path and never for a step of
// another feature, which is the one thing here that is not read across the whole project: the cap
// and the file check are both the project's, and this one is the path's.
var ErrPredecessorNotDone = errors.New("store: the step this one waits for is not done")

// PredecessorError names the step this one waits for and the state that step is in.
//
// The state travels with the number because the operator's move depends on it: a step in flight is
// one to wait for, and a stopped step is one to rewrite the path around. A refusal saying only that
// the path is blocked leaves them with nothing to type.
type PredecessorError struct {
	// Number is the step this one waits for, in the same feature.
	Number int32
	// State is the state that step is in, and PredecessorMissing when the path holds no such step.
	State string
}

// PredecessorMissing is the state a predecessor reads as when the path holds no step of that number.
//
// The control plane refuses a path document whose After names a step the document does not have, so
// nothing reaching the store through it can land here. It is a word rather than a silent pass because
// a path that lost the step it waits for has to refuse the take rather than read as unblocked.
const PredecessorMissing = "not in this path"

func (e *PredecessorError) Error() string {
	return fmt.Sprintf("%s: step %d is %s", ErrPredecessorNotDone.Error(), e.Number, e.State)
}

// Is makes errors.Is(err, ErrPredecessorNotDone) answer, so a caller that only wants to know which
// rule refused the take does not have to unwrap the step.
func (e *PredecessorError) Is(target error) bool { return target == ErrPredecessorNotDone }

// ErrNothingRestated is returned when a step whose session wrote nothing is approved.
//
// The check and the write are one statement, for the reason ErrNothingToApprove is one: a read of
// the text followed by a write would let a step restated between the two come back approved under a
// text nobody read.
var ErrNothingRestated = errors.New("store: there is no restatement to approve")

// ErrRestatementNotApproved says nobody approved what the session wrote about this step, so the check
// refuses: a step whose restatement carries no approval has built nothing yet.
var ErrRestatementNotApproved = errors.New("store: nobody approved this step's restatement")

// ErrNoScenarioNamed says the step names no scenario, so there is nothing for krewe to run.
var ErrNoScenarioNamed = errors.New("store: this step names no scenario")

// ErrNotChecked says nothing ran this step's scenario yet, so the word done is refused. This is gate
// 3.
//
// It reads the moment of the last run and never the verdict. Nothing checked the step and the check
// said no are two states, and only the first one is refused: the word done belongs to the operator,
// so a step whose check failed still closes and the row records the disagreement.
var ErrNotChecked = errors.New("store: nobody read a verdict on this step yet")

// ErrNoProofCommand says the project carries no proof command, so krewe has nothing to run a scenario
// with.
var ErrNoProofCommand = errors.New("store: this project has no proof command")

// ErrNoOfferStanding says krewe offered this project nothing, so there is no offer to accept.
//
// The read of the offer and the write of the level are one statement, for the reason
// ErrNothingToApprove is one: a read followed by a write would let an offer cleared between the two
// come back accepted.
//
// A raise at the top level earns this same sentinel. Krewe never offers a level that does not exist,
// so a raise at level 1 is a raise against no offer however the row got there.
var ErrNoOfferStanding = errors.New("store: krewe offered this project no level")

// ErrNotClosedByKrewe says a reopen was asked for on a step that teaches krewe nothing to take back:
// one the operator closed, or one that is not done at all.
//
// The two are one sentinel because they are one rule from the side of the trust record. A reopen is
// the operator saying krewe was wrong, and a step krewe did not close says nothing about krewe. The
// control plane reads the row to tell a person which of the two they typed.
//
// The read of the state and the write are one statement, for the reason ErrNothingToApprove is one: a
// read followed by a write would let a step closed between the two be reopened against a row nobody
// read.
var ErrNotClosedByKrewe = errors.New("store: krewe did not close this step")

// DefaultStepsInFlightCap is how many steps a project nobody configured may hold in state taken at
// one time. It is the default of the column, repeated here for the stores to answer with when a
// project carries no design row at all.
//
// Ten is an observation rather than a target. On 9 September 2026 this project held 10 steps in state
// taken at one moment, on feature 1, counted by grouping feature_steps through features onto the
// project; weft and rex each held 1. The number is that count and nothing else: it is not tuned, and
// it says nothing about how many sessions a person can read at once. It is what the system was
// already running, so the cap refuses no work already being done. The design document records a
// default of 3 and calls it a guess that nothing measured, and that text is stale.
const DefaultStepsInFlightCap int32 = 10

// DefaultProofCountPattern and DefaultProofTimeoutSeconds are what a project nobody configured reads
// back for its proof settings. They are the defaults of their columns, repeated here for the same
// reason DefaultStepsInFlightCap is: a project with no design row has to answer what the row would
// have given it, or a reader learns one thing before the first write and another after it.
//
// The pattern is the shape the godog runner prints, and the group is what a later slice reads the
// number out of. Nine hundred seconds is fifteen minutes.
const (
	DefaultProofCountPattern         = `([0-9]+) scenarios`
	DefaultProofTimeoutSeconds int32 = 900
)

// ProofSettings is what one scenario run looks like in a project: the command, how to read the count
// of scenarios out of what it printed, and the budget for one run.
//
// An empty value leaves that setting where it is. Empty Command, empty CountPattern and a
// TimeoutSeconds of zero each mean "do not change this one", so a caller may set the command alone
// without losing the pattern the project already had. There is no way to clear one back to nothing,
// and none is needed: a proof command is replaced rather than removed.
type ProofSettings struct {
	Command        string
	CountPattern   string
	TimeoutSeconds int32
}

// DefaultTrustThreshold is the run of agreements a project nobody configured needs before krewe is
// offered the next level. It is the default of the column, repeated here for the reason
// DefaultStepsInFlightCap is: a project with no design row has to answer what the row would have
// given it.
//
// Five is a guess and nothing measured it. The project sets its own, and the command that prints the
// record says the number is provisional.
const DefaultTrustThreshold int32 = 5

// The two levels the word done sits at, and the only two numbers trust_level ever holds.
//
// At TrustLevelChecked krewe checks a step and the operator says done. At TrustLevelCloses krewe
// closes a step its own check passed. A disagreement takes the level back down, and it never goes
// below the first or above the second.
const (
	TrustLevelChecked int32 = 0
	TrustLevelCloses  int32 = 1
)

// The two words operator_agreed is one of. A step nobody closed carries neither, which is the empty
// string, so a reader can tell a step nothing is decided about from a step the operator differed on.
const (
	AgreedYes = "yes"
	AgreedNo  = "no"
)

// ClosedByKrewe is the word closed_by carries when krewe closed a step its own check passed.
//
// It is here because the store reads it rather than only writing it: a reopen is allowed on a step
// krewe closed and refused on every other, so both stores have to refuse the same rows. The control
// plane writes the word from this constant for the same reason, because a word one layer spelled for
// itself would refuse every reopen in the system.
const ClosedByKrewe = "krewe"

// Agreed says whether the operator's word matched what krewe's last run of the scenario reported.
//
// It is read from the row and never asked. Done after a passing check agrees with krewe, and stopped
// after a failing check agrees with it too: both say the operator read the verdict and did what it
// pointed at. Everything else is a disagreement, including a step closed with nothing run on it,
// because krewe reported nothing to agree with.
//
// Nothing asks the operator. That is a question with an obvious answer and one more keystroke.
//
// The Postgres store says this same thing in the statement that writes the state, because the write
// is one statement there. The conformance suite holds the two to one answer.
func Agreed(state, proofState string) string {
	if state == StepDone && proofState == ProofPassing {
		return AgreedYes
	}
	if state == StepStopped && proofState == ProofFailing {
		return AgreedYes
	}
	return AgreedNo
}

// LoweredTrustLevel is the level a disagreement leaves a project at. It falls by one while the level
// is above zero, and a disagreement at level 0 records the disagreement and leaves the level alone.
//
// The floor is here rather than in each store, for the reason TakeableStates is: two stores held to
// one conformance suite cannot each own a bound, or a level that went negative in one of them would
// read as a third level to everything downstream.
func LoweredTrustLevel(level int32) int32 {
	if level <= TrustLevelChecked {
		return TrustLevelChecked
	}
	return level - 1
}

// OfferTheNextLevel reports whether a finish that just moved the counters earns krewe an offer: the
// run of agreements reached the project's threshold, and the level is below the top one.
//
// It is here rather than in each store, for the reason LoweredTrustLevel is: two stores held to one
// conformance suite cannot each own the rule, or a project would be offered the word done on one of
// them and never on the other.
//
// A disagreement never reaches it. The caller asks only where the operator agreed, because a
// disagreement takes the offer away whatever the numbers read.
//
// It answers about a run the write has already made. Setting the threshold below the run a project
// already has makes no offer by itself: nothing calls this except a finish.
func OfferTheNextLevel(run, threshold, level int32) bool {
	return run >= threshold && level < TrustLevelCloses
}

// The three words a step's proof state is one of. A step nobody checked is unproven, which is what
// every step is born as, and the other two are what a run reported.
//
// They are named here because the store writes them and four other places read them, and a word one
// of those spelled for itself would read as a step that never passed anything.
const (
	ProofUnproven = "unproven"
	ProofPassing  = "passing"
	ProofFailing  = "failing"
)

// ProofOutputKept is how much of a run's output a step keeps: the last 4,000 characters, because the
// reason a run failed is at the end of it. A run that printed more carries one line above them saying
// how much was dropped, so nobody reads a cut output as the whole of it.
const ProofOutputKept = 4_000

// ProofResult is what one run of the scenario reported: the verdict, how many scenarios ran, and what
// the run printed.
//
// The store keeps what it is given. Which run passes is the control plane's rule, and a second
// judgement here would be a second place for it to drift.
type ProofResult struct {
	State        string
	ScenariosRun int32
	Output       string
}

// KeptProofOutput is the output as a step keeps it: whole while it fits, and otherwise the last
// ProofOutputKept characters under one line saying how many came before them.
//
// Both stores call it, so the two cannot disagree about what a long run reads back as. The line is
// above the output rather than below it, because a reader who stops after one line has to learn that
// what follows is the end of a run and not the whole of it.
func KeptProofOutput(output string) string {
	if len(output) <= ProofOutputKept {
		return output
	}
	cut := len(output) - ProofOutputKept
	return fmt.Sprintf("[the first %d characters of this run were cut, the last %d are kept]\n%s",
		cut, ProofOutputKept, output[cut:])
}

// StepInFlight is one step in state taken, and the feature it sits in. The feature travels with it
// because the cap counts across the whole project: a refusal naming step 1 three times, in a project
// where three features each hold one, tells the operator nothing about which to finish.
type StepInFlight struct {
	Number        int32
	FeatureNumber int32
	FeatureTitle  string
	// Touches is what this step says it writes, one file per line, as the path document wrote it.
	// It travels with the step because the take reads it in the same statement it counts them in: a
	// second read for the files would be a second moment, and a take that landed between the two
	// would be missed.
	Touches string
}

// SharedFileError names the file two steps write and the step in flight that already writes it.
//
// One file and one step rather than every collision, because the operator has one move either way:
// wait for that step, or finish it. FLIGHT-3 says which one it is, so two runs of one take name the
// same step.
type SharedFileError struct {
	// File is the line both steps carry, trimmed of the spaces at each end.
	File string
	// Step is the step in flight that names it, and the feature that step sits in.
	Step StepInFlight
}

func (e *SharedFileError) Error() string {
	return fmt.Sprintf("%s: step %d.%d %s writes %s",
		ErrStepsTouchTheSameFile.Error(), e.Step.FeatureNumber, e.Step.Number, e.Step.FeatureTitle, e.File)
}

// Is makes errors.Is(err, ErrStepsTouchTheSameFile) answer, so a caller that only wants to know which
// rule refused the take does not have to unwrap the file.
func (e *SharedFileError) Is(target error) bool { return target == ErrStepsTouchTheSameFile }

// FilesTouched is what a step says it writes: the touches text split on newlines, each line trimmed
// of the spaces at each end, with the empty lines dropped.
//
// It is here rather than in each store because the two stores are held to one conformance suite, and
// a trim that ran in one of them would let the same path collide in Postgres and pass in memory.
func FilesTouched(touches string) []string {
	files := make([]string, 0)
	for _, line := range strings.Split(touches, "\n") {
		if named := strings.TrimSpace(line); named != "" {
			files = append(files, named)
		}
	}
	return files
}

// SharesAFile is the first file the step being taken names that a step in flight names too, or nil
// when the two sets are apart.
//
// The steps in flight are read in feature number order and then step number order, and the answer is
// the first match in that walk, so the same take refused twice names the same step. The comparison is
// exact and case sensitive: this reads text a person wrote in a document, and ./internal/store.go and
// internal/store.go are two lines to it.
func SharesAFile(touches string, flying []StepInFlight) *SharedFileError {
	taking := make(map[string]bool)
	for _, file := range FilesTouched(touches) {
		taking[file] = true
	}
	for _, held := range flying {
		for _, file := range FilesTouched(held.Touches) {
			if taking[file] {
				return &SharedFileError{File: file, Step: held}
			}
		}
	}
	return nil
}

// StepsInFlightError names every step in flight and the cap they were counted against.
//
// All of them rather than a count, because the operator's next move is to finish one, and a refusal
// that says only how many sends them to the listing to work out which.
type StepsInFlightError struct {
	// Steps are the steps in state taken, by feature number and then step number.
	Steps []StepInFlight
	// Cap is the number they were counted against.
	Cap int32
}

func (e *StepsInFlightError) Error() string {
	said := make([]string, 0, len(e.Steps))
	for _, step := range e.Steps {
		said = append(said, fmt.Sprintf("step %d.%d %s", step.FeatureNumber, step.Number, step.FeatureTitle))
	}
	return fmt.Sprintf("%s: %d in flight against a cap of %d: %s",
		ErrTooManyStepsInFlight.Error(), len(e.Steps), e.Cap, strings.Join(said, ", "))
}

// Is makes errors.Is(err, ErrTooManyStepsInFlight) answer, so a caller that only wants to know which
// rule refused the take does not have to unwrap the steps.
func (e *StepsInFlightError) Is(target error) bool { return target == ErrTooManyStepsInFlight }

// StepDone is the state a step moves to when the work in it is finished, and StepStopped the state
// it moves to when somebody abandons it. They are here for the reason StepReady is: a path write
// reads the word to decide whether the step is protected, so both stores must read the same word.
const (
	StepDone    = "done"
	StepStopped = "stopped"
)

// TakeableStates are the two states a step may be taken from: nobody took it yet, or somebody took it
// and stopped it. A take from either one starts the step clean.
//
// Taken and done are the two that are refused. A step somebody holds is refused so two sessions never
// build one step, and a step that closed is refused so the record of finished work stands.
//
// They are here rather than in each store because the two stores are held to one conformance suite,
// and a state one of them allowed would let the same take pass in memory and refuse in Postgres.
func TakeableStates() []string { return []string{StepReady, StepStopped} }

// Takeable says whether a step in this state may be taken.
func Takeable(state string) bool {
	for _, word := range TakeableStates() {
		if state == word {
			return true
		}
	}
	return false
}

// protectedStepStates are the three states that hold a record of work. A step in one of them was
// taken by somebody, so a document that drops or renames it takes that record away.
//
// Ready is not one of them. Every ready step is replaced whole, or a path could never be corrected.
func protectedStepStates() []string { return []string{StepTaken, StepDone, StepStopped} }

// ProtectedStep is one step a path write would have lost, and the state that protects it.
type ProtectedStep struct {
	Number int32
	State  string
}

// ProtectedStepsError names every protected step one path write would have dropped or renamed. It
// carries all of them rather than the first, because a document that lost two steps sends the
// operator back twice when the refusal names one.
type ProtectedStepsError struct {
	// Steps are the protected steps, in number order.
	Steps []ProtectedStep
}

func (e *ProtectedStepsError) Error() string {
	said := make([]string, 0, len(e.Steps))
	for _, step := range e.Steps {
		said = append(said, fmt.Sprintf("step %d is %s", step.Number, step.State))
	}
	return ErrPathHoldsTakenSteps.Error() + ": " + strings.Join(said, ", ")
}

// Is makes errors.Is(err, ErrPathHoldsTakenSteps) answer, so a caller that only wants to know which
// rule refused the write does not have to unwrap the numbers.
func (e *ProtectedStepsError) Is(target error) bool { return target == ErrPathHoldsTakenSteps }

// protectedSteps names the steps of the path as it stands that the incoming document would drop or
// rename, in number order.
//
// A step is dropped when the document holds no step of that number, and renamed when it holds that
// number under a different title. Both take the record away: the number then names work nobody did,
// and a session reading the path is told to build something else under it.
func protectedSteps(held []*quaycrewv1.Step, incoming []Step) []ProtectedStep {
	titles := make(map[int32]string, len(incoming))
	for _, step := range incoming {
		titles[step.Number] = step.Title
	}
	lost := make([]ProtectedStep, 0)
	for _, step := range held {
		if !slices.Contains(protectedStepStates(), step.GetState()) {
			continue
		}
		if title, kept := titles[step.GetNumber()]; kept && title == step.GetTitle() {
			continue
		}
		lost = append(lost, ProtectedStep{Number: step.GetNumber(), State: step.GetState()})
	}
	sort.Slice(lost, func(i, j int) bool { return lost[i].Number < lost[j].Number })
	return lost
}

// keepTheRecord carries what the system owns from the step as it stands onto the step the document
// declares: the state, the session that took it, the result, who closed it, the stamps, what the
// session restated and what the last run of its scenario reported.
//
// The document is what a caller may set, and none of these are on it. A write that took them from
// the document would let somebody declare work that never happened, and one that left them behind
// would lose the work that did.
func keepTheRecord(writing, held *quaycrewv1.Step) {
	writing.State = held.GetState()
	writing.Session = held.GetSession()
	writing.Result = held.GetResult()
	writing.ClosedBy = held.GetClosedBy()
	writing.TakenAt = held.GetTakenAt()
	writing.FinishedAt = held.GetFinishedAt()
	writing.Restatement = held.GetRestatement()
	writing.RestatedAt = held.GetRestatedAt()
	writing.RestatementApproved = held.GetRestatementApproved()
	writing.RestatementApprovedAt = held.GetRestatementApprovedAt()
	writing.ProofState = held.GetProofState()
	writing.ProofScenariosRun = held.GetProofScenariosRun()
	writing.ProofOutput = held.GetProofOutput()
	writing.ProofRanAt = held.GetProofRanAt()
}

// Step is what a caller may set about one step of a path.
//
// The rest of the row belongs to the system: the state, the session that took it, the result and the
// stamps are written by taking, finishing and stopping a step, never by the document that declares
// the path. A caller that could set them would be declaring work it had not done.
type Step struct {
	Number        int32
	Title         string
	Intention     string
	Touches       string
	Proof         string
	ProofScenario string
	After         int32
	// Milestone is which milestone of the feature this step belongs to, and zero when it belongs to
	// none. It is a caller's to set because it comes off the document that declares the path, beside
	// the number and the title.
	Milestone int32
	// Contracts is the contracts this step builds, one identifier per line, and ContractScope is what
	// part of each one is this step's, one line per contract reading `<identifier>: <sentence>`.
	//
	// The store keeps both as the caller wrote them. Whether an identifier names a contract that
	// exists is a question about a document krewe never reads, so nothing here asks it.
	Contracts     string
	ContractScope string
}

// Finish is what closes a step: the word, what came of it, and who spoke the word.
//
// The result is required at the control plane rather than here, for the reason the state word is:
// the store keeps what it is given, and one place refuses.
type Finish struct {
	// State is done or stopped.
	State string
	// Result is what the step produced, or why it stopped.
	Result string
	// ClosedBy is operator or krewe.
	ClosedBy string
}

// Milestone is what a caller may set about one milestone of a feature's path.
//
// A milestone holds nothing else. What it reached is counted from the steps under it, so there is no
// state here for a caller to set and none for the store to keep.
type Milestone struct {
	Number    int32
	Title     string
	Intention string
}

// StepReady is the state a step is born in, and the same word the table's own default writes. It is
// here for the reason StatusReclaimed is: the memory store writes it directly and must not disagree
// with the column, and the store cannot depend on the package that owns the vocabulary.
const StepReady = "ready"

// StepTaken is the state a step moves to when a session takes it, and it is here for the reason
// StepReady is: both stores write the word directly and must not disagree about it.
const StepTaken = "taken"

// FeatureOpen is the state a feature is born in, and the same word the table's own default writes. It
// is here for the reason StepReady is: the memory store writes it directly and must not disagree with
// the column.
const FeatureOpen = "open"

// StatusReclaimed is the session status this store writes when the system takes a container back. The
// control plane owns the whole vocabulary; this one is here because two queries below are written in
// terms of it and the store must not depend on the package that calls it.
const StatusReclaimed = "reclaimed"

// holdingStatuses are the states a session can be in, still hold a container, and still be nothing's
// to hold open: waiting for job, or holding a failed last exec.
//
// "running" is absent because an exec is in flight. "stopped" is absent because an operator put the
// session down, and a system that reclaimed what somebody halted would be overwriting a decision with
// bookkeeping. "reclaimed" is absent because the container has already gone, and mixing it in here is
// what starved the reclaim: see IdleSandboxes.
func holdingStatuses() []string { return []string{"idle", "failed"} }

// StatusRunning is the session status that means an exec is in flight. Both stores refuse to archive
// a session carrying it, and the refusal is a condition on the write, so the word has to be here for
// the same reason StatusReclaimed is: the store cannot depend on the package that owns the
// vocabulary, and the two must not disagree about the spelling.
const StatusRunning = "running"

// settledStatuses are the states a session holds no container in, which is what the whole project
// sweep puts away. A session in any other state is left in the listing rather than refused: the sweep
// names a project, not a session, so it takes what it can and reports what it left.
func settledStatuses() []string { return []string{"stopped", "failed", StatusReclaimed} }

// ErrSkillChanged is returned when a version of a skill is imported again carrying a different skill.
//
// It is a refusal rather than an overwrite because a workspace pins the version it holds. Overwriting
// would change a skill under sessions already using it, silently, which is exactly what pinning is
// for. The way forward is to raise the version in the manifest.
var ErrSkillChanged = errors.New("store: that version of the skill is already imported and differs")

// Imported is a skill the system holds: the skill as its author wrote it, and when it came in.
type Imported struct {
	skill.Skill
	ImportedAt time.Time
}

// ErrHookChanged is returned when a version of a hook is imported again carrying a different hook.
//
// A refusal rather than an overwrite, and the stakes are higher here than for a skill: overwriting
// would change a constraint under sessions already running under it, which is how a gate quietly
// stops gating. The way forward is to raise the version in the manifest.
var ErrHookChanged = errors.New("store: that version of the hook is already imported and differs")

// ImportedHook is a hook the system holds: the hook as its author wrote it, and when it came in.
type ImportedHook struct {
	hook.Hook
	ImportedAt time.Time
}

// Birth is what is true of a session only at the moment it is made.
//
// Both fields are read once, when the row is written, and ignored for a session that already exists.
// A session's sandbox is born with its capabilities and never drifts, and these are the same
// statement one level up: changing the system's configuration must not widen a conversation already
// running, and a role's boundary must hold for every exec of the session rather than for the first.
type Birth struct {
	// Mode is what the session's execs may do without asking, from the system's configuration. An
	// unknown or empty one is the mode every session had before this was configurable, so a system
	// that says nothing does not change under an upgrade.
	Mode string
	// Title is what to call the session, from whoever dispatched it, so the name is on the row before
	// the first exec runs. Empty for a caller that has no name for the conversation yet.
	Title string
}

// SessionFilter narrows a listing. The zero value is every live session the system has.
type SessionFilter struct {
	// Project wins over Workspace when both are set, being the narrower.
	Workspace string
	Project   string
	// Archived asks for the sessions put away instead of the live ones, never both: the default view
	// must not quietly grow back the sessions somebody hid.
	Archived bool
}

// Store persists workspaces, channels and sessions. Workspaces are soft deleted, so the sessions
// that reference one keep their history.
type Store interface {
	CreateWorkspace(ctx context.Context, name string) (*quaycrewv1.Workspace, error)
	GetWorkspace(ctx context.Context, id string) (*quaycrewv1.Workspace, error)
	ListWorkspaces(ctx context.Context) ([]*quaycrewv1.Workspace, error)
	DeleteWorkspace(ctx context.Context, id string) error
	AttachChannel(ctx context.Context, workspace, id, kind string) (*quaycrewv1.Channel, error)

	CreateProject(ctx context.Context, workspace, name string) (*quaycrewv1.Project, error)
	GetProject(ctx context.Context, id string) (*quaycrewv1.Project, error)
	// ListProjects lists every project, or one workspace's when workspace is set.
	ListProjects(ctx context.Context, workspace string) ([]*quaycrewv1.Project, error)
	DeleteProject(ctx context.Context, id string) error
	// SetDeployTarget records where a project ships, and a zero target clears it. The store keeps
	// what it is given: whether a target is whole, and whether its identity belongs to its account,
	// is the control plane's question.
	SetDeployTarget(ctx context.Context, project string, target deploy.Target) error
	// SetProjectRepository records where a project's work lands, and what kind of repository that is.
	// Writing it again replaces what is held, so a project that moved repository is corrected rather
	// than growing a second answer.
	SetProjectRepository(ctx context.Context, project, repository, visibility string) (*quaycrewv1.Project, error)

	// FindOrCreateSession creates on first use, so a channel that knows only its own session id always
	// lands in the same session.
	//
	// born is what is true of the session only when it is made. It is ignored for a session that
	// already exists: what a session may do and who it works as are its own, and neither may change
	// under a conversation already running.
	//
	// It says whether it made one. Only the store knows, and the caller has to: a session coming into
	// existence is an event, and finding out afterwards means comparing timestamps and guessing.
	FindOrCreateSession(ctx context.Context, project, session string, born Birth) (found *quaycrewv1.Session, created bool, err error)
	// SetSessionSkills records the skill set a session's live sandbox was born with; empty clears
	// it. SessionSkills reads it back, empty when no live sandbox is known. Stopping or archiving
	// a session clears it, because the sandbox goes with it and the next one is born current.
	SetSessionSkills(ctx context.Context, id, fingerprint string) error
	SessionSkills(ctx context.Context, id string) (string, error)
	// FindOrCreateDriver returns the project's driver, one per project, creating it on first open.
	FindOrCreateDriver(ctx context.Context, project string) (*quaycrewv1.Session, error)
	// RecordExec leaves the stored handle alone when modelSessionID is empty, so a failed exec cannot
	// erase it.
	RecordExec(ctx context.Context, id, modelSessionID, status string) error
	GetSession(ctx context.Context, id string) (*quaycrewv1.Session, error)
	// ListSessions returns sessions last moved first: put away if they were, last touched otherwise,
	// which is the clock a listing's age column shows. The order is decided here rather than by each
	// surface, so the console, the command line and the web page cannot drift apart.
	ListSessions(ctx context.Context, filter SessionFilter) ([]*quaycrewv1.Session, error)
	StopSession(ctx context.Context, id string) error
	// ReclaimSession records that the system took a session's container back: the status becomes
	// reclaimed and the moment is stamped. Nothing else moves. The conversation handle, the workspace's
	// conversation store and the project's files are all untouched, so the next exec builds a fresh
	// container over the same state and the conversation carries on.
	//
	// It is not a stop. A stop is somebody's decision, and a session that went quiet must never read
	// the same as one that was halted. Whether a session is in a state that may be reclaimed is the
	// control plane's question, not the store's.
	ReclaimSession(ctx context.Context, id string) error
	// IdleSandboxes is the sessions that still hold a container and nothing is holding open, oldest
	// touched first: live, not running, not already reclaimed, and named by no job in a non terminal
	// phase. It is the fourth query a controller runs each tick, and the one a reclaim acts on.
	//
	// A session is not a second resource with a declaration of its own. What is wanted of it is read
	// from the job that names it, so job still in flight keeps its session alive and nothing here
	// has to be told.
	//
	// Reclaimed rows are left out, and that is the whole reason this is not one query with the one
	// below. A reclaimed session has no container left to take, and with no archive time set it stays
	// settled for ever, so a single batch fills with rows nothing can move and never reaches a
	// sandbox. Two queries, each in its own order, and neither can starve the other.
	IdleSandboxes(ctx context.Context, limit int) ([]*quaycrewv1.Session, error)
	// ReclaimedSessions is the sessions whose container has already gone and that nothing is holding
	// open, longest reclaimed first. It is what the archive time is measured against, so it is ordered
	// by reclaimed_at rather than by updated_at: a reclaim writes both, and only one of them says how
	// long the session has been in this state.
	ReclaimedSessions(ctx context.Context, limit int) ([]*quaycrewv1.Session, error)
	// ArchiveSession only hides a session from the default listing. The row, the conversation handle
	// and the files on the host all stay.
	//
	// A session whose status is running is refused with ErrSessionHoldsAnExec, and a session that is
	// archived already changes no row and is ErrNotFound. The refusal is a condition on the update
	// rather than a read before it, so nothing can start an exec between the two.
	//
	// The write clears the skills fingerprint. The sandbox goes with the archive, so the fingerprint
	// of the skills that sandbox was born with describes nothing.
	ArchiveSession(ctx context.Context, id string) error
	// ArchiveProjectSessions puts away every session of one project that holds no container, and
	// returns what it stamped, in the order the archived listing draws them.
	//
	// It skips a session that holds one rather than refusing it. The single form refuses, because the
	// operator named that session; this form names a project, and a sweep that stops at the first live
	// session finishes nothing. A session already archived is not stamped again and is not returned.
	//
	// It is one statement, for the reason ArchiveSession is: a dispatch that lands during the sweep
	// either runs before the statement and keeps its session out of it, or runs after it.
	ArchiveProjectSessions(ctx context.Context, project string) ([]string, error)
	// RestoreSession brings an archived session back into the default listing.
	RestoreSession(ctx context.Context, id string) error
	// SetPermissionMode records what a session's execs may do without asking. Whether the mode is one
	// the model understands is the control plane's question, not the store's.
	SetPermissionMode(ctx context.Context, id, mode string) error
	// SetDescription records what the system observed a session to be, and how many execs it had when
	// that was written, so the two can never disagree about how current the description is.
	SetDescription(ctx context.Context, id, description string, atExec int) error
	// CountExecs is how many execs a session has had, which is what decides whether a description has
	// fallen behind the conversation.
	CountExecs(ctx context.Context, session string) (int, error)
	// SetLabel records what the operator calls a session. Empty clears it, which is the only way back
	// to the identifier, so it is a value rather than an absence.
	SetLabel(ctx context.Context, id, label string) error
	// RestartSession marks a stopped session idle again. The conversation is untouched, because it
	// lives on the host rather than in the sandbox that was torn down, which is the whole reason
	// bringing a session back is possible at all. Whether the session was stopped in the first place
	// is the control plane's question, not the store's.
	RestartSession(ctx context.Context, id string) error

	// GetContext returns what the model should be told at a scope, and empty when nothing has been
	// written there. Nothing written is the normal state and is not an error.
	GetContext(ctx context.Context, scope ContextScope, owner string) (string, error)
	// SetContext records what the model should be told at a scope.
	SetContext(ctx context.Context, scope ContextScope, owner, body string) error

	// GetDesign returns what a project is for and what was designed for it. A project that exists
	// with no design row answers with a Design carrying its identifier and nothing else, the way
	// GetContext answers an unwritten level: nothing written is the normal state and is not an error.
	// The body comes back whole, because a design read short is a design read wrong.
	GetDesign(ctx context.Context, project string) (*quaycrewv1.Design, error)
	// SetProjectBrief records what a project is for, and creates the row on first use. An empty brief
	// clears it, which makes the brief a value rather than an absence.
	SetProjectBrief(ctx context.Context, project, brief string) (*quaycrewv1.Design, error)
	// SetProjectDesign records the design document whole, and creates the row on first use.
	// writtenBy is the session that wrote it and is empty when the operator did; it is a claim the
	// system keeps rather than one it checks. No length refuses a body.
	//
	// The same write clears the approval, because approval is a statement about one text. One write
	// rather than two, so no reader ever sees a row that says approved over a body nobody read.
	SetProjectDesign(ctx context.Context, project, body, writtenBy string) (*quaycrewv1.Design, error)
	// SetProjectContracts records the contracts document whole, and creates the row on first use. It
	// is a second body beside the design, and writtenBy is a claim the same way.
	//
	// It leaves the approval where it is. The operator's word is about the design body, and the
	// contracts are read from that body, so a contract written down is not a design that changed.
	//
	// An empty body is kept: it is how a project says it carries no contracts document.
	SetProjectContracts(ctx context.Context, project, body, writtenBy string) (*quaycrewv1.Design, error)
	// ApproveProjectDesign records the operator's word on the design as it stands, and refuses a
	// design with no body as ErrNothingToApprove. Approving one that is already approved is allowed
	// and moves the moment.
	ApproveProjectDesign(ctx context.Context, project string) (*quaycrewv1.Design, error)
	// SetStepsInFlightCap records how many steps of one project may be in state taken at one time,
	// and creates the design row on first use.
	//
	// The store keeps what it is given. Whether a number is one a person should have typed is the
	// control plane's question, the way a permission mode already is.
	//
	// Lowering it below what runs now is allowed and stops nothing: it refuses the next take. A cap
	// that reached into running sessions would end work nobody asked it to end.
	SetStepsInFlightCap(ctx context.Context, project string, atOnce int32) (*quaycrewv1.Design, error)
	// SetProofCommand records what one scenario run looks like in this project, and creates the
	// design row on first use.
	//
	// The store keeps what it is given. Whether the command names a scenario, whether the pattern
	// compiles and whether the budget is one a person should have typed are the control plane's
	// questions, the way the cap above already is.
	//
	// An empty value in ProofSettings leaves that setting where it is, so a caller sets the command
	// alone without losing the pattern the project already had.
	//
	// The approval is untouched, and so is every trust column. A proof command says how a step is
	// run, and nothing about what the design says.
	SetProofCommand(ctx context.Context, project string, settings ProofSettings) (*quaycrewv1.Design, error)
	// RaiseTrust accepts the offer krewe made and returns the design after the level moves. The write
	// adds one to trust_level, sets trust_run to zero and takes the offer away.
	//
	// A project with no offer standing is ErrNoOfferStanding, and so is a raise at the top level: the
	// offer is only ever made below it, so a raise there is a raise against nothing. A project that
	// does not exist, and one carrying no design row, are both ErrNotFound.
	//
	// The read of the offer and the write are one statement. Read first and written after, an offer
	// cleared by a disagreement landing between the two would come back accepted.
	//
	// Krewe never calls it. The offer is krewe's and the word that accepts it is the operator's, which
	// is why DeniedToDriver names this call.
	RaiseTrust(ctx context.Context, project string) (*quaycrewv1.Design, error)
	// SetTrustThreshold records the run of agreements that earns an offer of the next level, and
	// creates the design row on first use.
	//
	// The store keeps what it is given. Whether a number is one a person should have typed is the
	// control plane's question, the way the cap above already is.
	//
	// No counter moves. Changing the threshold is neither an agreement nor a disagreement, and a
	// threshold set below the run a project already has makes no offer by itself: the offer is made
	// where a finish moves the run.
	SetTrustThreshold(ctx context.Context, project string, threshold int32) (*quaycrewv1.Design, error)

	// SetPath replaces one feature's path and returns the whole path after the write, in number
	// order. The steps are what a caller may set; the rest of each row belongs to the system.
	//
	// It touches no other feature of the same project, in any case, refusal included. Keyed by the
	// project, a second path wiped the first.
	//
	// The store keeps what it is given. Whether a number is unique, whether `after` names a step
	// that exists, and whether a title says anything are the control plane's questions, because the
	// document is where a person can be told which line is wrong.
	//
	// The milestones and the steps are written in one transaction, from the one document that
	// declares both. Writing them apart would let a step name a milestone the same write dropped.
	//
	// A step that is taken, done or stopped is protected: a document that drops it, or holds its
	// number under another title, is refused as ErrPathHoldsTakenSteps and writes nothing. A
	// protected step the document keeps takes its title, intention, touches, proof, scenario,
	// milestone and contracts from the document, and keeps the state, the session, the result and
	// the stamps the system wrote on it.
	SetPath(ctx context.Context, feature string, milestones []Milestone, steps []Step) ([]*quaycrewv1.Step, error)
	// ListSteps returns a feature's path in number order, or every feature's when the identifier is
	// empty, ordered by feature and then by number. A feature with no path is an empty slice and not
	// an error, the way a project with no design is.
	ListSteps(ctx context.Context, feature string) ([]*quaycrewv1.Step, error)
	// ListMilestones returns a feature's milestones in number order. A feature with no milestone is
	// an empty slice and not an error, the way one with no path is.
	//
	// Nothing writes a milestone except SetPath, so this read always agrees with the last path
	// document.
	ListMilestones(ctx context.Context, feature string) ([]*quaycrewv1.Milestone, error)
	// GetStep returns one step of a feature's path, whole. A feature that does not exist and a path
	// that holds no step of that number are both ErrNotFound: neither answers the question asked.
	GetStep(ctx context.Context, feature string, number int32) (*quaycrewv1.Step, error)
	// TakeStep gives a step to a session and returns the step after the write, with how many steps of
	// the project are in state taken once it lands. A step that is neither ready nor stopped is
	// ErrStepNotReady, and the caller reads the step to say who holds it.
	//
	// A stopped step may be taken again, and it starts clean: the write sets the proof state back to
	// unproven and clears the restatement, its approval and every proof column. A second attempt
	// proves itself again rather than inheriting the first attempt's verdict, and an approval carried
	// over would let the session past the gate that reads one.
	//
	// The count is the write's own, so a caller that prints it prints what the take made rather than
	// what a second read a moment later says.
	//
	// Three rules refuse a take. ErrPredecessorNotDone reads the state of the step this one waits for,
	// inside this feature, and refuses while that step is not done. ErrTooManyStepsInFlight counts
	// every step in state taken in the whole project against the design's cap. ErrStepsTouchTheSameFile
	// reads what each of those steps says it writes, and refuses a step that names a file one of them
	// names.
	//
	// The two limits read the whole project and the predecessor reads one feature. Counted inside the
	// feature, two features could each run the whole cap, and two features could each write one file at
	// the same moment. Read across the project, a step would wait for a step of a path it has nothing
	// to do with, and the two features could not run at once at all.
	//
	// Several steps may be taken at once, in one feature or across the features of one project.
	//
	// The predecessor, the count, the file check, the state check and the write are one transaction, so
	// two callers cannot both take one step, two takes at one moment cannot both pass a cap with room
	// for one of them, and two takes at one moment cannot both pass a file only one of them may write.
	TakeStep(ctx context.Context, feature string, number int32, session string) (*quaycrewv1.Step, int32, error)
	// FinishStep records what came of a step: the word that closes it, what somebody wrote, who spoke
	// the word, and the stamp. A feature that does not exist and a path that holds no step of that
	// number are both ErrNotFound.
	//
	// The store writes the state it is given. Whether done and stopped are the only two words is the
	// control plane's question, the way a permission mode already is.
	//
	// The word done is refused with ErrNotChecked while the step carries no moment of a run. This is
	// gate 3, and it reads that moment rather than the verdict: a failing run is a record, and the
	// row keeps the disagreement rather than refusing the word. A stop reads nothing at all, because
	// a step nobody will finish has to be closable whatever ran on it.
	//
	// The same write records whether the operator agreed with krewe's last verdict, and the design
	// row of the project holding this feature moves its counters in the same transaction. Agreed says
	// what agreement is. No reader ever sees a closed step whose counters did not move, which is why
	// the design comes back from this call rather than from a read after it.
	//
	// The offer rides on the same write. An agreement that takes the run to the project's threshold
	// sets trust_offered, and a disagreement takes it away, because the run went back to zero and an
	// offer that survived what invalidated it is worse than no offer. OfferTheNextLevel holds the
	// rule, and this is the only call that makes one.
	//
	// The session and the take stamp are untouched, so the record still says who took the step. The
	// step and the session are separate records, and nothing here reads or writes a session.
	FinishStep(ctx context.Context, feature string, number int32, finish Finish) (
		*quaycrewv1.Step, *quaycrewv1.Design, error)

	// ReopenStep takes a step back off krewe: the state goes back to taken, the row records that the
	// operator differed, and the project's trust level falls by one. A feature that does not exist and
	// a path that holds no step of that number are both ErrNotFound.
	//
	// It is allowed only on a step in state done whose closed_by is ClosedByKrewe. Every other row is
	// ErrNotClosedByKrewe, a step the operator closed as much as a step nobody closed: nothing about
	// trust is learned from the operator disagreeing with the operator.
	//
	// why is written into result, over whatever krewe wrote there, so the row says what was wrong
	// while the step stays taken. The store keeps what it is given, and whether a why says anything is
	// the control plane question, the way the two words a step ends with already are.
	//
	// The design row moves in the same transaction, through the counters a finish moves, so a reopen
	// counts exactly as any other disagreement: the level falls by one, the run goes to zero, the
	// disagreements gain one, and no reader ever sees a reopened step whose level did not fall.
	//
	// The session, the take stamp, the proof columns and the restatement columns are untouched. The
	// operator answers the same conversation with an ordinary exec, and a reopen that cleared the
	// restatement would make the session prove itself again for a fault of the checker.
	ReopenStep(ctx context.Context, feature string, number int32, why string) (
		*quaycrewv1.Step, *quaycrewv1.Design, error)

	// SetRestatement records what the session wrote about a step before it built anything, and
	// returns the step after the write. A feature that does not exist and a path that holds no step
	// of that number are both ErrNotFound.
	//
	// The same write clears the approval and its stamp. Approval is a statement about one text, so a
	// step whose restatement changed is a step nobody has agreed to yet.
	//
	// Writing the same text again is allowed, and it still clears the approval: the store cannot tell
	// an unchanged text from a rewritten one that reads the same. The caller skips the call when the
	// text did not change, which is where that saving belongs.
	//
	// No length refuses the text. A cap here would lose work that exists only in the call being made.
	SetRestatement(ctx context.Context, feature string, number int32, text string) (*quaycrewv1.Step, error)
	// ApproveRestatement records the operator's word on the restatement as it stands, and returns the
	// step after the write. A step whose session wrote nothing is ErrNothingRestated. A feature that
	// does not exist and a path that holds no step of that number are both ErrNotFound.
	//
	// The text is read in the statement that writes the approval, so a step restated between a read
	// and a write cannot come back approved under a text nobody read.
	//
	// Approving one that is already approved is allowed, and it moves the stamp. Nothing about the
	// proof columns moves: what a session understood and what a run reported are two records.
	ApproveRestatement(ctx context.Context, feature string, number int32) (*quaycrewv1.Step, error)
	// RecordProof records what one run of the step's scenario reported, and returns the step after
	// the write. A feature that does not exist and a path that holds no step of that number are both
	// ErrNotFound.
	//
	// The four proof columns are written together, whatever the result. A failing run is a record and
	// not a gap, and the moment is stamped on a failing run too, so the gate that refuses a finish
	// before anybody read a verdict opens either way.
	//
	// The output is kept as KeptProofOutput keeps it: whole while it fits, and otherwise the last
	// characters under a line saying how many came before them.
	//
	// The state written is the one the caller computed. The store judges nothing about a run, and a
	// second judgement here would be a second place for the rule to drift.
	RecordProof(ctx context.Context, feature string, number int32, result ProofResult) (*quaycrewv1.Step, error)

	// ListFeatures returns a project's features in number order, or every project's when the
	// identifier is empty, ordered by project and then by number. A project with no feature is an
	// empty slice and not an error, the way a project with no path is. Every state comes back:
	// filtering to the open ones is the caller's question.
	ListFeatures(ctx context.Context, project string) ([]*quaycrewv1.Feature, error)
	// GetFeature returns one feature, whole. A feature that does not exist, and one of a deleted
	// project, are both ErrNotFound.
	//
	// The step calls take a feature and the design, its approval and the cap all belong to the
	// project, so something has to say which project a feature sits in. This is it.
	GetFeature(ctx context.Context, feature string) (*quaycrewv1.Feature, error)
	// AddFeature gives a project one more narrowed part of itself, numbered the highest number in
	// that project plus one, and returns it.
	//
	// The store gives the number and a caller never chooses one. The read of the highest number and
	// the insert are one statement, so two callers at one moment cannot take the same number, and a
	// number is never reused: a feature that stopped keeps it.
	AddFeature(ctx context.Context, project string, title string) (*quaycrewv1.Feature, error)
	// SetFeatureIntention records which part of the project a feature narrows to, in one line, and
	// returns the feature after the write. It touches no other column. An empty intention is kept: a
	// feature that says nothing yet is the normal state.
	SetFeatureIntention(ctx context.Context, feature string, intention string) (*quaycrewv1.Feature, error)
	// FinishFeature writes a feature's state and returns the feature after the write.
	//
	// The store keeps the word it is given and judges none of them, so a word outside the three is
	// refused at the control plane and the check lives in one place. Setting the state back to open
	// goes through here too, which is why there is no separate method for reopening.
	//
	// It touches no step and no milestone. A closed feature keeps its whole path, and a feature may be
	// closed while steps under it are ready or taken: the operator decides when a feature is finished.
	FinishFeature(ctx context.Context, feature string, state string) (*quaycrewv1.Feature, error)

	// ImportSkill takes a skill into the system at the version its manifest declares.
	//
	// Importing the same name and version again is fine when it is the same skill and refused when it
	// is not, because a workspace pins a version: a version that changed underneath it would be a
	// skill changing under a session already using it, which is the one thing pinning exists to stop.
	ImportSkill(ctx context.Context, imported Imported) error
	// GetSkill returns one revision of a skill, its files included.
	GetSkill(ctx context.Context, name string, version int) (Imported, error)
	// ListSkills returns the newest revision of every skill the system holds, without their files. A
	// listing is read to see what exists, and the files are the largest part of a skill.
	ListSkills(ctx context.Context) ([]Imported, error)
	// AttachSkill gives a workspace a skill, pinned to the newest revision the system holds now.
	// Attaching one it already holds moves it to that revision.
	AttachSkill(ctx context.Context, workspace, name string) (Imported, error)
	// DetachSkill takes a skill away from a workspace. The skill stays imported, because another
	// workspace may hold it and because importing it again should not be the price of a change of mind.
	DetachSkill(ctx context.Context, workspace, name string) error
	// WorkspaceSkills returns the skills a workspace holds, at the versions it pinned, files included,
	// which is what a sandbox needs to be built.
	WorkspaceSkills(ctx context.Context, workspace string) ([]Imported, error)
	// AttachSystemSkill gives the skill to the whole system, pinned to the newest revision the system holds
	// now, so every workspace has it and a workspace made tomorrow has it too. Attaching again is how
	// the system moves to a newer revision.
	AttachSystemSkill(ctx context.Context, name string) (Imported, error)
	// DetachSystemSkill takes a skill away from the system. A workspace that attached it for itself keeps
	// it: the two are separate statements, and the narrower one is not undone by the wider one.
	DetachSystemSkill(ctx context.Context, name string) error
	// SystemSkills returns what the system holds, at the versions it pinned, files included.
	SystemSkills(ctx context.Context) ([]Imported, error)

	// The same six questions again for hooks. A hook is the same kind of thing as a skill, authored
	// as files and attached at a level, and it is a separate set of calls rather than one generic
	// set because the two entities are separate: a session refused a skill still runs, and a session
	// refused a hook is a session running without the constraint it was supposed to have.
	//
	// ImportHook takes a hook into the system at the version its manifest declares. The same name and
	// version again is fine when it is the same hook and refused when it is not, for the reason
	// ImportSkill gives.
	ImportHook(ctx context.Context, imported ImportedHook) error
	// GetHook returns one revision of a hook, its files included.
	GetHook(ctx context.Context, name string, version int) (ImportedHook, error)
	// ListHooks returns the newest revision of every hook the system holds, without their files.
	ListHooks(ctx context.Context) ([]ImportedHook, error)
	// AttachHook gives a workspace a hook, pinned to the newest revision the system holds now.
	AttachHook(ctx context.Context, workspace, name string) (ImportedHook, error)
	// DetachHook takes a hook away from a workspace. The hook stays imported.
	DetachHook(ctx context.Context, workspace, name string) error
	// WorkspaceHooks returns the hooks a workspace holds, at the versions it pinned, files included,
	// which is what a sandbox needs to be built.
	WorkspaceHooks(ctx context.Context, workspace string) ([]ImportedHook, error)
	// AttachSystemHook gives the hook to the whole system, so every workspace has it and one made
	// tomorrow has it too. This is the level most hooks want: a constraint the system agreed on is not
	// usually a per workspace opinion.
	AttachSystemHook(ctx context.Context, name string) (ImportedHook, error)
	// DetachSystemHook takes a hook away from the system. A workspace that attached it for itself keeps
	// it: the two are separate statements, and the narrower one is not undone by the wider one.
	DetachSystemHook(ctx context.Context, name string) error
	// SystemHooks returns what the system holds, at the versions it pinned, files included.
	SystemHooks(ctx context.Context) ([]ImportedHook, error)

	// AppendExec records one exec of a session's history, and is safe to call twice with the same
	// exec: a caller retrying a write it is not sure landed must leave one exec, so a record it has
	// already written must not double it. The exec's Id is what makes that possible.
	AppendExec(ctx context.Context, exec *quaycrewv1.Exec, workspace, project, session string) error
	// FinishExec writes what came of an exec into the record its start opened, and leaves the rest
	// of that record alone. An exec is written when it starts, so what a session was asked is
	// visible while it works; this is the other half of that, the same row closed.
	//
	// An exec the store does not hold is not an error. The exec itself already happened, and the
	// operator has its result, so a missing row must not come back as a failure of the exec.
	FinishExec(ctx context.Context, id, status, reply, failure string) error

	// AppendSessionEvent records one thing that happened to a session, and is safe to call twice with
	// the same event for the same reason AppendExec is: the event's Id is what makes a repeat harmless.
	AppendSessionEvent(ctx context.Context, event *quaycrewv1.SessionEvent) error
	// ListSessionEvents returns a session's lifecycle oldest first, capped at limit, so it reads the
	// way it happened. An empty session asks for the whole system's, which is what a view of what is
	// going on right now reads. A limit of zero or less means the default.
	ListSessionEvents(ctx context.Context, session string, limit int) ([]*quaycrewv1.SessionEvent, error)
	// What reads back the pull requests the crew opened. UnsettledPullRequests is the job whose pull
	// request is still worth reading, longest unread first, and RecordPullRequest keeps what the forge
	// said. Neither is a movement of the job: the job ended when it ended, and what happened to the
	// work afterwards happened on the forge.

	// ListExecs returns a session's history oldest first, capped at limit, so a conversation reads
	// the way it happened. A limit of zero or less means the default.
	ListExecs(ctx context.Context, session string, limit int) ([]*quaycrewv1.Exec, error)

	// Probe writes, so a caller can prove the store still takes one. A system whose reads all answer
	// and whose writes never land looks healthy from every listing, which is how a control plane that
	// dispatched nothing went unnoticed for an hour. It writes one row and keeps writing over it, so
	// asking often costs one row and never grows.
	Probe(ctx context.Context) error

	// Close releases whatever the implementation holds open.
	Close()
}

// ContextScope is which level a piece of context belongs to. They layer: the system's is true
// everywhere, a workspace's inside it, a project's inside that.
type ContextScope string

const (
	// ContextSystem is true of everything this system does. Its owner is empty.
	ContextSystem ContextScope = "system"
	// ContextWorkspace is true of one workspace, owned by its id.
	ContextWorkspace ContextScope = "workspace"
	// ContextProject is true of one project, owned by its id.
	ContextProject ContextScope = "project"
	// ContextSession is true of one conversation, owned by its session id. It is the innermost level,
	// and where a note written from inside a sandbox lands.
	ContextSession ContextScope = "session"
)

// ContextLevels are the scopes in order, outermost first. They layer: everything the system knows, then
// the workspace, then the project, then this one conversation.
func ContextLevels() []ContextScope {
	return []ContextScope{ContextSystem, ContextWorkspace, ContextProject, ContextSession}
}

// KnownContextScope says whether a scope is one of the three.
func KnownContextScope(scope ContextScope) bool {
	switch scope {
	case ContextSystem, ContextWorkspace, ContextProject, ContextSession:
		return true
	default:
		return false
	}
}

// NewID returns a random identifier for a workspace or a session.
func NewID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// NewConversationID returns an identifier for a conversation with the model.
//
// It is a version 4 identifier rather than one of ours, because the model's command line tool is
// given it with `--session-id` and rejects anything that is not one. The system chooses it rather than
// reading it back afterwards: a conversation started interactively never tells anybody what it picked,
// so every conversation opened from the panel was one the system could not name, could not show a
// history for, and could not count the tokens of.
func NewConversationID() string {
	return uuid.NewString()
}

// sortByLastMoved orders a session listing newest movement first, which is the clock its last column
// shows: when a session was put away if it was, and when it was last touched otherwise.
//
// It sorts on the same stamp the listing renders because the two used to disagree. The order was the
// created stamp and the column was this one, so a session made a week ago and used an hour ago sat
// below one made yesterday and untouched since, and a real listing of forty five sessions read
// 1d, 1d, 1d, 7d, 7d, 7d, 1d, 7d. A column in no order is a column nobody can read.
//
// The identifier breaks a tie, so two sessions that share a moment keep one order between reads.
//
// Postgres writes the same rule as `coalesce(archived_at, updated_at) desc, id`, and storetest holds
// the two to it. The stamp comes from internal/session, which is also where the age column reads it,
// so the order and the cell can never be computed from different fields.
func sortByLastMoved(sessions []*quaycrewv1.Session) {
	sort.Slice(sessions, func(i, j int) bool {
		left, right := session.LastMoved(sessions[i]).AsTime(), session.LastMoved(sessions[j]).AsTime()
		if left.Equal(right) {
			return sessions[i].GetId() < sessions[j].GetId()
		}
		return left.After(right)
	})
}
