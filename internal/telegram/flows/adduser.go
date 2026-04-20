package flows

import (
	"sync"
	"time"
)

// AddUserStep identifies where in the /adduser wizard an admin is.
type AddUserStep string

const (
	StepAwaitID      AddUserStep = "await_id"
	StepAwaitName    AddUserStep = "await_name"
	StepAwaitConfirm AddUserStep = "await_confirm"
)

// AddUserFlow is one admin's in-flight /adduser session.
type AddUserFlow struct {
	Step     AddUserStep
	TargetID int64
	Name     string
	Token    string
	Expires  time.Time
}

// AddUserFlows maps admin-id → active wizard, with a TTL sweep.
type AddUserFlows struct {
	mu      sync.Mutex
	ttl     time.Duration
	byAdmin map[int64]*AddUserFlow
}

func NewAddUserFlows(ttl time.Duration) *AddUserFlows {
	return &AddUserFlows{ttl: ttl, byAdmin: map[int64]*AddUserFlow{}}
}

// Start resets any existing flow for this admin and returns a fresh one.
func (f *AddUserFlows) Start(adminID int64) *AddUserFlow {
	f.mu.Lock()
	defer f.mu.Unlock()
	fl := &AddUserFlow{Step: StepAwaitID, Expires: time.Now().Add(f.ttl)}
	f.byAdmin[adminID] = fl
	return fl
}

// Current returns the active, non-expired flow for this admin, or nil.
func (f *AddUserFlows) Current(adminID int64) *AddUserFlow {
	f.mu.Lock()
	defer f.mu.Unlock()
	fl := f.byAdmin[adminID]
	if fl == nil {
		return nil
	}
	if time.Now().After(fl.Expires) {
		delete(f.byAdmin, adminID)
		return nil
	}
	return fl
}

// Cancel clears the admin's flow.
func (f *AddUserFlows) Cancel(adminID int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.byAdmin, adminID)
}

// Advance mutates the admin's flow under lock and extends TTL.
func (f *AddUserFlows) Advance(adminID int64, mutate func(*AddUserFlow)) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	fl := f.byAdmin[adminID]
	if fl == nil || time.Now().After(fl.Expires) {
		delete(f.byAdmin, adminID)
		return false
	}
	mutate(fl)
	fl.Expires = time.Now().Add(f.ttl)
	return true
}

// ClaimByToken returns the flow iff it's in awaitConfirm and matches token.
func (f *AddUserFlows) ClaimByToken(adminID int64, token string) *AddUserFlow {
	f.mu.Lock()
	defer f.mu.Unlock()
	fl := f.byAdmin[adminID]
	if fl == nil || time.Now().After(fl.Expires) {
		delete(f.byAdmin, adminID)
		return nil
	}
	if fl.Step != StepAwaitConfirm || fl.Token != token {
		return nil
	}
	return fl
}

// Sweep drops expired flows.
func (f *AddUserFlows) Sweep() {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	for id, fl := range f.byAdmin {
		if now.After(fl.Expires) {
			delete(f.byAdmin, id)
		}
	}
}

// NewToken mints a short random token for the confirm step. Exposed so
// callers (the /adduser wizard) and the flow itself agree on format.
func NewToken() string { return newToken() }
