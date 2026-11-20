package hive

import "sort"

// Briefing gathers every role's report into what the operator reads first:
// actions to take, decisions to make, what is blocked, who is working where,
// notices, then each role's status.
type Briefing struct {
	// Actions are the operator's steps, grouped by kind in ActionKinds
	// order, then by role.
	Actions []RoleAction `json:"actions"`
	// Decisions are the open decision requests, by role then ID.
	Decisions []RoleDecision `json:"decisions"`
	// Answered are decision requests the decision log already answers; the
	// role should read the decision and drop the request from its report.
	Answered []RoleDecision `json:"answered"`
	// Blocked are the blockers, those waiting on the operator first.
	Blocked []RoleBlocker `json:"blocked"`
	// InProgress is the work in progress, by repository.
	InProgress []RepositoryWork `json:"inProgress"`
	// Notices are what roles told the others, by role.
	Notices []RoleNotice `json:"notices"`
	// Reports are the reports from current roles, by role.
	Reports []Report `json:"reports"`
	// Idle are the roles that reported they have nothing to do.
	Idle []string `json:"idle"`
	// Missing are active roles that have not reported.
	Missing []string `json:"missing"`
	// Ignored are reports from roles that do not exist or are retired.
	Ignored []string `json:"ignored"`
}

// RoleAction is an operator action and the role that asked for it.
type RoleAction struct {
	Role   string `json:"role"`
	Action Action `json:"action"`
}

// RoleDecision is a decision request and the role that raised it. Key,
// "<role>/<id>", is unique across the briefing.
type RoleDecision struct {
	Key     string          `json:"key"`
	Role    string          `json:"role"`
	Request DecisionRequest `json:"request"`
}

// RoleBlocker is a blocker and the role it blocks.
type RoleBlocker struct {
	Role    string  `json:"role"`
	Blocker Blocker `json:"blocker"`
}

// RoleWork is work in progress and the role doing it.
type RoleWork struct {
	Role string `json:"role"`
	Work Work   `json:"work"`
}

// RepositoryWork is the work in progress in one repository. Shared is set
// when more than one role is working there.
type RepositoryWork struct {
	Repository string     `json:"repository"`
	Shared     bool       `json:"shared"`
	Work       []RoleWork `json:"work"`
}

// RoleNotice is a notice and the role that raised it.
type RoleNotice struct {
	Role   string `json:"role"`
	Notice Notice `json:"notice"`
}

// NewBriefing builds a briefing from the roles, the decision log and the
// current reports.
func NewBriefing(roles []Role, decisions []Decision, reports []Report) Briefing {
	b := Briefing{
		Actions: []RoleAction{}, Decisions: []RoleDecision{}, Answered: []RoleDecision{},
		Blocked: []RoleBlocker{}, InProgress: []RepositoryWork{}, Notices: []RoleNotice{},
		Reports: []Report{}, Idle: []string{}, Missing: []string{}, Ignored: []string{},
	}
	byName := map[string]Role{}
	for _, r := range roles {
		byName[r.Name] = r
	}
	decided := map[string]bool{}
	for _, d := range decisions {
		decided[d.ID] = true
	}
	reported := map[string]bool{}
	work := map[string][]RoleWork{}
	for _, r := range reports {
		if role, ok := byName[r.Role]; !ok || role.Status == RoleRetired {
			b.Ignored = append(b.Ignored, r.Role)
			continue
		}
		b.Reports = append(b.Reports, r)
		reported[r.Role] = true
		if r.Idle {
			b.Idle = append(b.Idle, r.Role)
		}
		for _, a := range r.OperatorActions {
			b.Actions = append(b.Actions, RoleAction{Role: r.Role, Action: a})
		}
		for _, d := range r.DecisionsNeeded {
			rd := RoleDecision{Key: r.Role + "/" + d.ID, Role: r.Role, Request: d}
			if decided[d.ID] {
				b.Answered = append(b.Answered, rd)
			} else {
				b.Decisions = append(b.Decisions, rd)
			}
		}
		for _, bl := range r.Blocked {
			b.Blocked = append(b.Blocked, RoleBlocker{Role: r.Role, Blocker: bl})
		}
		for _, w := range r.InProgress {
			work[w.Repository] = append(work[w.Repository], RoleWork{Role: r.Role, Work: w})
		}
		for _, n := range r.Notices {
			b.Notices = append(b.Notices, RoleNotice{Role: r.Role, Notice: n})
		}
	}
	sort.Slice(b.Reports, func(i, j int) bool { return b.Reports[i].Role < b.Reports[j].Role })
	kindRank := map[ActionKind]int{}
	for i, k := range ActionKinds {
		kindRank[k] = i
	}
	sort.SliceStable(b.Actions, func(i, j int) bool {
		ki, kj := kindRank[b.Actions[i].Action.Kind], kindRank[b.Actions[j].Action.Kind]
		if ki != kj {
			return ki < kj
		}
		return b.Actions[i].Role < b.Actions[j].Role
	})
	byKey := func(s []RoleDecision) func(i, j int) bool {
		return func(i, j int) bool { return s[i].Key < s[j].Key }
	}
	sort.SliceStable(b.Decisions, byKey(b.Decisions))
	sort.SliceStable(b.Answered, byKey(b.Answered))
	sort.SliceStable(b.Blocked, func(i, j int) bool {
		oi, oj := b.Blocked[i].Blocker.WaitingOn == Operator, b.Blocked[j].Blocker.WaitingOn == Operator
		if oi != oj {
			return oi
		}
		return b.Blocked[i].Role < b.Blocked[j].Role
	})
	for repo, ws := range work {
		sort.SliceStable(ws, func(i, j int) bool { return ws[i].Role < ws[j].Role })
		shared := false
		for _, w := range ws[1:] {
			shared = shared || w.Role != ws[0].Role
		}
		b.InProgress = append(b.InProgress, RepositoryWork{Repository: repo, Shared: shared, Work: ws})
	}
	sort.Slice(b.InProgress, func(i, j int) bool {
		if b.InProgress[i].Shared != b.InProgress[j].Shared {
			return b.InProgress[i].Shared
		}
		return b.InProgress[i].Repository < b.InProgress[j].Repository
	})
	sort.SliceStable(b.Notices, func(i, j int) bool { return b.Notices[i].Role < b.Notices[j].Role })
	for _, r := range roles {
		if r.Status == RoleActive && !reported[r.Name] {
			b.Missing = append(b.Missing, r.Name)
		}
	}
	sort.Strings(b.Idle)
	sort.Strings(b.Missing)
	sort.Strings(b.Ignored)
	return b
}
