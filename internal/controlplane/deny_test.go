package controlplane_test

import (
	"strings"
	"testing"

	quaycrewv1 "github.com/atlantic-blue/quay-krewe/gen/quaycrew/v1"
	"github.com/atlantic-blue/quay-krewe/internal/controlplane"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The driver is refused a named list and holds everything else, so a call added tomorrow is open to it
// unless somebody says otherwise. The list is the capability grants: what a session may not do is give
// itself or anybody else more capability than it was dispatched with.

// A hook is a command that runs on a session's own tool use, so a session that could attach one
// would change what every session in that workspace may do.
func TestTheDriverMayNotTouchAHook(t *testing.T) {
	for _, method := range []string{
		quaycrewv1.ControlPlaneService_ImportHook_FullMethodName,
		quaycrewv1.ControlPlaneService_ListHooks_FullMethodName,
		quaycrewv1.ControlPlaneService_AttachHook_FullMethodName,
		quaycrewv1.ControlPlaneService_DetachHook_FullMethodName,
	} {
		err := controlplane.DeniedToDriver(method, nil)
		if status.Code(err) != codes.PermissionDenied {
			t.Errorf("%s answered %v, want PermissionDenied", method, status.Code(err))
		}
	}
}

// What the driver exists to do stays open, or the deny list would be a system that cannot be driven.
func TestTheDriverStillDoesWhatItExistsToDo(t *testing.T) {
	for _, method := range []string{
		quaycrewv1.ControlPlaneService_Dispatch_FullMethodName,
		quaycrewv1.ControlPlaneService_CreateWorkspace_FullMethodName,
		quaycrewv1.ControlPlaneService_CreateProject_FullMethodName,
		quaycrewv1.ControlPlaneService_ListSkills_FullMethodName,
	} {
		if err := controlplane.DeniedToDriver(method, nil); err != nil {
			t.Errorf("%s was refused to the driver: %v", method, err)
		}
	}
}

// The word on a design stage is the operator's, and the six stages are the design. A session that
// could approve one would agree with the text it wrote itself, which is the gate this call exists
// behind.
func TestTheDriverMayNotApproveADesignStage(t *testing.T) {
	err := controlplane.DeniedToDriver(
		quaycrewv1.ControlPlaneService_ApproveDesignStage_FullMethodName, nil)
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("approving a design stage answered %v, want PermissionDenied", status.Code(err))
	}
	if message := status.Convert(err).Message(); !strings.Contains(message, "ApproveDesignStage") {
		t.Errorf("the refusal %q does not name the call", message)
	}
}

// Writing a stage and reading the six stay open, because a design session is what writes them. A
// deny list that took those would leave the stages with nobody to fill them in.
func TestTheDriverStillWritesAndReadsTheStages(t *testing.T) {
	for _, method := range []string{
		quaycrewv1.ControlPlaneService_SetDesignStage_FullMethodName,
		quaycrewv1.ControlPlaneService_ListDesignStages_FullMethodName,
	} {
		if err := controlplane.DeniedToDriver(method, nil); err != nil {
			t.Errorf("%s was refused to the driver: %v", method, err)
		}
	}
}
