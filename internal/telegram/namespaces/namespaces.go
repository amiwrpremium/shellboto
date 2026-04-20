// Package namespaces defines every callback-data namespace and action
// used by the Telegram bot. All button constructions and every
// b.RegisterHandler for a callback go through CBData / CBPrefix here, so
// there are no raw "dc:" / "pr:y:" strings scattered across handler code.
package namespaces

import "strconv"

// NS is a typed callback-data namespace prefix.
type NS string

const (
	Danger  NS = "dc" // dangerous-command confirm
	Job     NS = "j"  // running command (cancel/kill)
	AddUser NS = "au" // /adduser wizard
	Promote NS = "pr" // /promote flow
	Demote  NS = "dm" // /demote flow
	Users   NS = "us" // /users browser
)

// Action is a typed sub-action within a namespace. Letters can repeat
// across namespaces (e.g. "c" is cancel in `j:` and commands-view in `us:`)
// — the parser is always namespace-scoped, so there's no collision at
// runtime. Named constants make the intent clear at each call site.
type Action string

const (
	// Shared across multiple namespaces.
	Select Action = "s"
	Yes    Action = "y"
	No     Action = "n"
	List   Action = "l"

	// Job (j:) actions.
	JobCancel Action = "c"
	JobKill   Action = "k"

	// Users browser (us:) actions.
	Profile      Action = "p"
	Audit        Action = "a"
	UsrCommands  Action = "c" // same letter as JobCancel, different namespace
	Output       Action = "o"
	Remove       Action = "r"
	Reinstate    Action = "i"
	RemoveYes    Action = "rY"
	ReinstateYes Action = "iY"
)

// CBPrefix returns the registration prefix for a namespace: e.g. "pr:".
func CBPrefix(ns NS) string { return string(ns) + ":" }

// CBData builds a callback-data string. If an id is supplied, it's appended
// as the third segment: "pr:y:12345". Without an id: "pr:n".
func CBData(ns NS, act Action, id ...int64) string {
	base := string(ns) + ":" + string(act)
	if len(id) > 0 {
		base += ":" + strconv.FormatInt(id[0], 10)
	}
	return base
}

// CBDataToken is the 3-segment form with a string token instead of an id,
// used by flows that mint non-numeric tokens (danger confirm, adduser).
func CBDataToken(ns NS, act Action, token string) string {
	return string(ns) + ":" + string(act) + ":" + token
}
