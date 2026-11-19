package hive

import "sort"

// Briefing gathers every role's report into what the operator reads first:
// decisions to make, then what is blocked, then each role's status.
type Briefing struct {
	// Decisions are the open decision requests, by role then ID.
	Decisions []RoleDecision `json:"decisions"`
	// Blocked are the blockers, those waiting on the operator first.
	Blocked []RoleBlocker `json:"blocked"`
	// Reports are the reports, by role.
	Reports []Report `json:"reports"`
	// Missing are active roles that have not reported.
	Missing []string `json:"missing"`
}

// RoleDecision is a decision request and the role that raised it.
type RoleDecision struct {
	Role    string          `json:"role"`
	Request DecisionRequest `json:"request"`
}

// RoleBlocker is a blocker and the role it blocks.
type RoleBlocker struct {
	Role    string  `json:"role"`
	Blocker Blocker `json:"blocker"`
}

// NewBriefing builds a briefing from the current reports.
func NewBriefing(roles []Role, reports []Report) Briefing {
	b := Briefing{Reports: append([]Report{}, reports...)}
	sort.Slice(b.Reports, func(i, j int) bool { return b.Reports[i].Role < b.Reports[j].Role })
	reported := map[string]bool{}
	for _, r := range b.Reports {
		reported[r.Role] = true
		for _, d := range r.DecisionsNeeded {
			b.Decisions = append(b.Decisions, RoleDecision{Role: r.Role, Request: d})
		}
		for _, bl := range r.Blocked {
			b.Blocked = append(b.Blocked, RoleBlocker{Role: r.Role, Blocker: bl})
		}
	}
	sort.SliceStable(b.Decisions, func(i, j int) bool {
		if b.Decisions[i].Role != b.Decisions[j].Role {
			return b.Decisions[i].Role < b.Decisions[j].Role
		}
		return b.Decisions[i].Request.ID < b.Decisions[j].Request.ID
	})
	sort.SliceStable(b.Blocked, func(i, j int) bool {
		oi, oj := b.Blocked[i].Blocker.WaitingOn == Operator, b.Blocked[j].Blocker.WaitingOn == Operator
		if oi != oj {
			return oi
		}
		return b.Blocked[i].Role < b.Blocked[j].Role
	})
	for _, r := range roles {
		if r.Status == RoleActive && !reported[r.Name] {
			b.Missing = append(b.Missing, r.Name)
		}
	}
	sort.Strings(b.Missing)
	if b.Decisions == nil {
		b.Decisions = []RoleDecision{}
	}
	if b.Blocked == nil {
		b.Blocked = []RoleBlocker{}
	}
	if b.Missing == nil {
		b.Missing = []string{}
	}
	return b
}
