package controlplane

import (
	"fmt"
	"strings"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// DeniedToDriver is the deny policy for the driver's token: the calls that grant capability are the
// operator's to make, so a session that can drive the system can never grant itself anything. Secrets
// because a secret becomes readable inside whatever sandbox it reaches; skills because a skill
// carries mounts, secret names and a setup that executes; a session's permission mode because
// loosening its own is the plainest self grant there is; and the system's context because it is
// injected into every session, including the driver itself.
//
// Importing a flow graph is refused for the same reason importing a skill is: writing an automation
// down is the operator deciding what the system may do on its own. Starting a run of one the operator
// already imported is not refused, because a run is dispatch, which the driver already has: it can
// reach nothing through a flow that it could not reach by dispatching directly.
//
// A role is refused on the same line as a skill. It carries a brief, a model and the material a
// session is allowed to receive, so a session that could import or attach one could write itself a
// way of working nobody approved and then be run as it. Reading what the system already holds stays
// open, because choosing from what the operator attached is the point.
//
// A hook is refused on all four of its calls, and the listing is the one place this differs from a
// skill. A hook is a command that runs on the session's own tool use, so attaching one changes what
// every session in that workspace may do, and reading the list is reading the map of the guard the
// session is under. A skill is a capability a session already holds and uses by name, so choosing
// from the ones the operator attached is the point of listing them.
//
// Approving a design is refused because the word is the operator's. A session may write a design,
// and writing one grants it nothing: the write clears the approval, so a session that wrote a design
// has produced a text somebody still has to read. A session that could approve its own text would be
// agreeing with itself, and the gate would be a step in a script rather than a person's judgement.
//
// Approving a restatement is refused on the same line as approving a design. The gate exists so that
// a person reads what a session understood before any code is built, and a session that could approve
// its own text would be agreeing with itself and dispatching itself to build. Writing a restatement
// stays open, because writing one grants nothing: it produces a text somebody still has to read.
//
// The cap on steps in flight is refused because it decides how much runs at once. A session that
// could raise its own would widen the fan out without anybody asking for it, and the operator would
// be reading more sessions than they agreed to. Taking a step stays open, because a take is a
// dispatch and the driver already has that: the cap is the number, not the act.
//
// The proof command is refused because it is what checks the session's own work. A session that
// could set it could point it at a scenario that always passes, or at nothing at all, and every step
// after that would report a check that checked nothing. The refusal is the whole reason the check is
// worth running. Reading the design stays open, so a session still reads the command it will be run
// under.
//
// The trust ladder is refused on both its calls, and they are the plainest self grant in the list:
// they move the word done. Krewe earns the next level by agreeing with the operator over and over, and
// it offers rather than takes, so a session that could raise its own would be handing itself the word
// it was supposed to earn. Reading the record stays open, because what a session is allowed to do is
// something it should be able to look up.
//
// Reopening a step is not refused, and it belongs to that same rule rather than standing against it.
// A reopen lowers the level by one and puts the work back, so a session that reopened its own step
// would be taking the word done away from itself. It grants nothing, and the operator reads the
// disagreement it records.
//
// Setting the threshold is refused beside it because lowering the number is the same grant by a
// longer road: a session that could say two agreements are enough would be writing its own offer.
// Raising the number grants nothing, and the call is refused whole rather than by the direction of
// the change, because a guard that reads which way a number moved is a guard nobody can check.
//
// Archiving is refused on both its calls. It is the operator's word about the record: a session that
// could put sessions away could hide the evidence of what it did, and the sweep over a project could
// do it to every finished session at once. Restoring stays open, because it hides nothing.
//
// Everything the driver exists to do stays open: workspaces, projects, sessions, dispatch, starting
// context at the workspace and project scopes, and reading or writing a design.
func DeniedToDriver(fullMethod string, request any) error {
	switch fullMethod {
	case quaycrewv1.ControlPlaneService_SetSecret_FullMethodName,
		quaycrewv1.ControlPlaneService_ListSecrets_FullMethodName,
		quaycrewv1.ControlPlaneService_ImportSkill_FullMethodName,
		quaycrewv1.ControlPlaneService_AttachSkill_FullMethodName,
		quaycrewv1.ControlPlaneService_DetachSkill_FullMethodName,
		quaycrewv1.ControlPlaneService_ImportHook_FullMethodName,
		quaycrewv1.ControlPlaneService_ListHooks_FullMethodName,
		quaycrewv1.ControlPlaneService_AttachHook_FullMethodName,
		quaycrewv1.ControlPlaneService_DetachHook_FullMethodName,
		quaycrewv1.ControlPlaneService_SetSessionPermissionMode_FullMethodName,
		quaycrewv1.ControlPlaneService_ArchiveSession_FullMethodName,
		quaycrewv1.ControlPlaneService_ArchiveProjectSessions_FullMethodName,
		quaycrewv1.ControlPlaneService_ApproveDesign_FullMethodName,
		quaycrewv1.ControlPlaneService_ApproveRestatement_FullMethodName,
		quaycrewv1.ControlPlaneService_SetStepsInFlightCap_FullMethodName,
		quaycrewv1.ControlPlaneService_SetProofCommand_FullMethodName,
		quaycrewv1.ControlPlaneService_RaiseTrust_FullMethodName,
		quaycrewv1.ControlPlaneService_SetTrustThreshold_FullMethodName:
		return refusedToDriver(fullMethod)
	case quaycrewv1.ControlPlaneService_SetContext_FullMethodName:
		if req, ok := request.(*quaycrewv1.SetContextRequest); ok && req.GetScope() == "system" {
			return refusedToDriver(fullMethod)
		}
	}
	return nil
}

func refusedToDriver(fullMethod string) error {
	name := shortMethod(fullMethod)
	return status.Error(codes.PermissionDenied, fmt.Sprintf(
		"the driver drives the system, it does not widen it: %s grants capability and is the operator's to make", name))
}

// shortMethod is the call's own name, without the service in front of it.
func shortMethod(fullMethod string) string {
	if i := strings.LastIndex(fullMethod, "/"); i >= 0 {
		return fullMethod[i+1:]
	}
	return fullMethod
}
