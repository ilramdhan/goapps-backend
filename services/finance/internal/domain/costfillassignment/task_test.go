package costfillassignment

import "testing"

func newTaskForTest(approver bool) *Task {
	rc := ResolvedConfig{
		RouteLevel: 7, FillerType: "DEPT", FillerValue: "RND",
		SLAFillHours: 48, SLAApproveHours: 24,
	}
	if approver {
		rc.ApproverType = "USER"
		rc.ApproverValue = "u-boss"
	}
	task := NewTask(100, 200, rc, 5)
	task.FilledParams = task.TotalParams // all params filled — ready to submit
	return task
}

func TestTask_ClaimThenFill_NoApprover_GoesApproved(t *testing.T) {
	task := newTaskForTest(false)
	if err := task.Claim("u-filler"); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if task.Status() != StatusFilling {
		t.Fatalf("want FILLING got %s", task.Status())
	}
	if err := task.Submit(); err != nil {
		t.Fatalf("submit: %v", err)
	}
	if task.Status() != StatusApproved {
		t.Fatalf("no-approver submit should go APPROVED, got %s", task.Status())
	}
}

func TestTask_Submit_WithApprover_GoesApprovalPending(t *testing.T) {
	task := newTaskForTest(true)
	_ = task.Claim("u-filler")
	_ = task.Submit()
	if task.Status() != StatusApprovalPending {
		t.Fatalf("want APPROVAL_PENDING got %s", task.Status())
	}
}

func TestTask_Approve_Reject_Cycle(t *testing.T) {
	task := newTaskForTest(true)
	_ = task.Claim("u-filler")
	_ = task.Submit()
	if err := task.Reject("u-boss", "fix qty"); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if task.Status() != StatusRejected {
		t.Fatalf("want REJECTED got %s", task.Status())
	}
	if err := task.Resubmit(); err != nil {
		t.Fatalf("resubmit: %v", err)
	}
	if task.Status() != StatusFilling {
		t.Fatalf("rejected resubmit should go FILLING got %s", task.Status())
	}
	_ = task.Submit()
	if err := task.Approve("u-boss"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if task.Status() != StatusApproved {
		t.Fatalf("want APPROVED got %s", task.Status())
	}
}

func TestTask_ReapproveOnChange(t *testing.T) {
	task := newTaskForTest(true)
	task.SetReapproveOnChange(true)
	_ = task.Claim("u-filler")
	_ = task.Submit()
	_ = task.Approve("u-boss")
	if err := task.MarkEditedAfterApproval(); err != nil {
		t.Fatalf("edit: %v", err)
	}
	if task.Status() != StatusApprovalPending {
		t.Fatalf("reapprove_on_change edit should go APPROVAL_PENDING got %s", task.Status())
	}
}
