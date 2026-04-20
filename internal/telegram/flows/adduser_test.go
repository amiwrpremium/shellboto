package flows

import (
	"testing"
	"time"
)

func TestAddUserFlowHappyPath(t *testing.T) {
	f := NewAddUserFlows(time.Minute)
	admin := int64(10)

	got := f.Start(admin)
	if got.Step != StepAwaitID {
		t.Fatalf("initial step = %q, want %q", got.Step, StepAwaitID)
	}

	f.Advance(admin, func(fl *AddUserFlow) {
		fl.TargetID = 42
		fl.Step = StepAwaitName
	})
	cur := f.Current(admin)
	if cur.TargetID != 42 || cur.Step != StepAwaitName {
		t.Fatalf("after advance: %+v", cur)
	}

	f.Advance(admin, func(fl *AddUserFlow) {
		fl.Name = "Alice Smith"
		fl.Token = "abcd1234"
		fl.Step = StepAwaitConfirm
	})
	if cl := f.ClaimByToken(admin, "abcd1234"); cl == nil || cl.Name != "Alice Smith" {
		t.Fatalf("ClaimByToken(valid) = %v", cl)
	}
	if cl := f.ClaimByToken(admin, "wrong"); cl != nil {
		t.Fatalf("ClaimByToken(wrong) = %+v", cl)
	}
	f.Cancel(admin)
	if f.Current(admin) != nil {
		t.Fatalf("Cancel left flow present")
	}
}

func TestAddUserFlowTTL(t *testing.T) {
	f := NewAddUserFlows(10 * time.Millisecond)
	f.Start(1)
	time.Sleep(25 * time.Millisecond)
	if f.Current(1) != nil {
		t.Fatalf("flow should have expired")
	}
}

func TestAddUserFlowStartResets(t *testing.T) {
	f := NewAddUserFlows(time.Minute)
	a := f.Start(7)
	a.TargetID = 11
	f.Advance(7, func(fl *AddUserFlow) { fl.Step = StepAwaitName })

	b := f.Start(7)
	if b.TargetID != 0 || b.Step != StepAwaitID {
		t.Fatalf("Start did not reset: %+v", b)
	}
}

func TestAddUserFlowClaimRequiresConfirm(t *testing.T) {
	f := NewAddUserFlows(time.Minute)
	f.Start(1)
	f.Advance(1, func(fl *AddUserFlow) {
		fl.Token = "xyz"
		fl.Step = StepAwaitName
	})
	if cl := f.ClaimByToken(1, "xyz"); cl != nil {
		t.Fatalf("claim should require await_confirm step")
	}
}

func TestAddUserFlowSweep(t *testing.T) {
	f := NewAddUserFlows(5 * time.Millisecond)
	f.Start(1)
	f.Start(2)
	time.Sleep(15 * time.Millisecond)
	f.Sweep()
	if f.Current(1) != nil || f.Current(2) != nil {
		t.Fatalf("sweep did not clear")
	}
}
