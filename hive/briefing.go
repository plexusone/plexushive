package hive

import (
	"slices"
	"sort"
	"strings"
)

// Briefing gathers every work identity's report into what the operator reads
// first: actions to take, decisions to make, what is blocked, who is working
// where, notices, then each work identity's status.
type Briefing struct {
	// Actions are the operator's steps, grouped by kind in ActionKinds order,
	// then by work identity. The same step asked for by several work identities
	// is listed once. A step that applies under an option of a decision the
	// operator chose against is left out.
	Actions []IdentityAction `json:"actions"`
	// Decisions are the open decision requests, by work identity then ID.
	Decisions []IdentityDecision `json:"decisions"`
	// Answered are decision requests the decision log already answers; the work
	// identity should read the decision and drop the request from its report.
	Answered []IdentityDecision `json:"answered"`
	// Blocked are the blockers, those waiting on the operator first.
	Blocked []IdentityBlocker `json:"blocked"`
	// InProgress is the work in progress, by repository.
	InProgress []RepositoryWork `json:"inProgress"`
	// Notices are what work identities told the others, by work identity.
	Notices []IdentityNotice `json:"notices"`
	// Reports are the reports from current identities, by work identity.
	Reports []Report `json:"reports"`
	// Idle are the work identities that reported they have nothing to do.
	Idle []string `json:"idle"`
	// Waiting are the work identities whose only open work waits on the
	// operator's answer to their decision requests.
	Waiting []string `json:"waiting"`
	// Missing are active work identities that have not reported.
	Missing []string `json:"missing"`
	// Ignored are reports from work identities that do not exist or are
	// retired.
	Ignored []string `json:"ignored"`
	// Unreadable are report files that could not be decoded; the caller
	// sets them from ReadReports with SetUnreadable.
	Unreadable []UnreadableReport `json:"unreadable"`
}

// IdentityAction is an operator action and the work identities that asked for
// it. Identity is the first of Identities. Condition, when set, is the open
// decision and option the action applies under, as "<identity>/<id> =
// <option>".
type IdentityAction struct {
	Identity   string   `json:"identity"`
	Identities []string `json:"identities"`
	Action     Action   `json:"action"`
	Condition  string   `json:"condition,omitempty"`
}

// IdentityDecision is a decision request and the work identity that raised it.
// Key, "<identity>/<id>", is unique across the briefing.
type IdentityDecision struct {
	Key      string          `json:"key"`
	Identity string          `json:"identity"`
	Request  DecisionRequest `json:"request"`
	// Option is the chosen option of an answered request, when known.
	Option string `json:"option,omitempty"`
}

// IdentityBlocker is a blocker and the work identity it blocks.
type IdentityBlocker struct {
	Identity string  `json:"identity"`
	Blocker  Blocker `json:"blocker"`
}

// IdentityWork is work in progress and the work identity doing it.
type IdentityWork struct {
	Identity string `json:"identity"`
	Work     Work   `json:"work"`
}

// RepositoryWork is the work in progress in one repository. Shared is set
// when more than one work identity is working there.
type RepositoryWork struct {
	Repository string         `json:"repository"`
	Shared     bool           `json:"shared"`
	Work       []IdentityWork `json:"work"`
}

// IdentityNotice is a notice and the work identity that raised it.
type IdentityNotice struct {
	Identity string `json:"identity"`
	Notice   Notice `json:"notice"`
}

// NewBriefing builds a briefing from the identities, the decision log and the
// current reports.
func NewBriefing(identities []WorkIdentity, decisions []Decision, reports []Report) Briefing {
	decided := map[string]Decision{}
	for _, d := range decisions {
		decided[d.ID] = d
	}
	return newBriefing(identities, func(_, id string) (string, bool) {
		d, ok := decided[id]
		return d.Option, ok
	}, reports)
}

// NewHiveBriefing builds a briefing from the hive and the reports. A
// request counts as answered when its open item has the operator's answer
// or a decision with its ID binds its work identity.
func NewHiveBriefing(h *Hive, reports []Report) Briefing {
	return newBriefing(h.Identities, h.Answered, reports)
}

// answeredFunc returns the option chosen for a work identity's request,
// and whether it has been answered.
type answeredFunc func(identity, id string) (string, bool)

func newBriefing(identities []WorkIdentity, answered answeredFunc, reports []Report) Briefing {
	b := Briefing{
		Actions: []IdentityAction{}, Decisions: []IdentityDecision{}, Answered: []IdentityDecision{},
		Blocked: []IdentityBlocker{}, InProgress: []RepositoryWork{}, Notices: []IdentityNotice{},
		Reports: []Report{}, Idle: []string{}, Waiting: []string{}, Missing: []string{}, Ignored: []string{},
		Unreadable: []UnreadableReport{},
	}
	byName := map[string]WorkIdentity{}
	for _, r := range identities {
		byName[r.Name] = r
	}
	actionAt := map[string]int{}
	reported := map[string]bool{}
	work := map[string][]IdentityWork{}
	for _, r := range reports {
		if identity, ok := byName[r.Identity]; !ok || identity.Status == IdentityRetired {
			b.Ignored = append(b.Ignored, r.Identity)
			continue
		}
		b.Reports = append(b.Reports, r)
		reported[r.Identity] = true
		if r.Idle {
			b.Idle = append(b.Idle, r.Identity)
		}
		if waitsOnlyOnDecisions(r, answered) {
			b.Waiting = append(b.Waiting, r.Identity)
		}
		for _, a := range r.OperatorActions {
			condition := ""
			if a.Decision != "" {
				option, ok := answered(r.Identity, a.Decision)
				switch {
				case ok && option != "" && a.Option != "" && option != a.Option:
					continue
				case !ok && a.Option != "":
					condition = r.Identity + "/" + a.Decision + " = " + a.Option
				}
			}
			key := actionKey(a)
			if i, ok := actionAt[key]; ok && key != "" {
				if !slices.Contains(b.Actions[i].Identities, r.Identity) {
					b.Actions[i].Identities = append(b.Actions[i].Identities, r.Identity)
				}
				continue
			}
			if key != "" {
				actionAt[key] = len(b.Actions)
			}
			b.Actions = append(b.Actions, IdentityAction{Identity: r.Identity, Identities: []string{r.Identity}, Action: a, Condition: condition})
		}
		for _, d := range r.DecisionsNeeded {
			rd := IdentityDecision{Key: r.Identity + "/" + d.ID, Identity: r.Identity, Request: d}
			if option, ok := answered(r.Identity, d.ID); ok {
				rd.Option = option
				b.Answered = append(b.Answered, rd)
			} else {
				b.Decisions = append(b.Decisions, rd)
			}
		}
		for _, bl := range r.Blocked {
			b.Blocked = append(b.Blocked, IdentityBlocker{Identity: r.Identity, Blocker: bl})
		}
		for _, w := range r.InProgress {
			work[w.Repository] = append(work[w.Repository], IdentityWork{Identity: r.Identity, Work: w})
		}
		for _, n := range r.Notices {
			b.Notices = append(b.Notices, IdentityNotice{Identity: r.Identity, Notice: n})
		}
	}
	sort.Slice(b.Reports, func(i, j int) bool { return b.Reports[i].Identity < b.Reports[j].Identity })
	kindRank := map[ActionKind]int{}
	for i, k := range ActionKinds {
		kindRank[k] = i
	}
	sort.SliceStable(b.Actions, func(i, j int) bool {
		ki, kj := kindRank[b.Actions[i].Action.Kind], kindRank[b.Actions[j].Action.Kind]
		if ki != kj {
			return ki < kj
		}
		return b.Actions[i].Identity < b.Actions[j].Identity
	})
	byKey := func(s []IdentityDecision) func(i, j int) bool {
		return func(i, j int) bool { return s[i].Key < s[j].Key }
	}
	sort.SliceStable(b.Decisions, byKey(b.Decisions))
	sort.SliceStable(b.Answered, byKey(b.Answered))
	sort.SliceStable(b.Blocked, func(i, j int) bool {
		oi, oj := slices.Contains(b.Blocked[i].Blocker.WaitingOn, Operator), slices.Contains(b.Blocked[j].Blocker.WaitingOn, Operator)
		if oi != oj {
			return oi
		}
		return b.Blocked[i].Identity < b.Blocked[j].Identity
	})
	for repo, ws := range work {
		sort.SliceStable(ws, func(i, j int) bool { return ws[i].Identity < ws[j].Identity })
		shared := false
		for _, w := range ws[1:] {
			shared = shared || w.Identity != ws[0].Identity
		}
		b.InProgress = append(b.InProgress, RepositoryWork{Repository: repo, Shared: shared, Work: ws})
	}
	sort.Slice(b.InProgress, func(i, j int) bool {
		if b.InProgress[i].Shared != b.InProgress[j].Shared {
			return b.InProgress[i].Shared
		}
		return b.InProgress[i].Repository < b.InProgress[j].Repository
	})
	sort.SliceStable(b.Notices, func(i, j int) bool { return b.Notices[i].Identity < b.Notices[j].Identity })
	for _, r := range identities {
		if r.Status == IdentityActive && !reported[r.Name] {
			b.Missing = append(b.Missing, r.Name)
		}
	}
	sort.Strings(b.Idle)
	sort.Strings(b.Waiting)
	sort.Strings(b.Missing)
	sort.Strings(b.Ignored)
	return b
}

// actionKey identifies the same step asked for by several work identities. A
// push to the same repository and branch is one step, however each work
// identity wrote the command, since one push sends every commit on the branch.
// Any other action is the same step when its kind, repository, branch and
// command match. Actions with neither are never merged.
func actionKey(a Action) string {
	if a.Kind == ActionPush && a.Repository != "" && a.Branch != "" {
		return strings.Join([]string{string(a.Kind), a.Repository, a.Branch}, "\x00")
	}
	cmd := strings.Join(strings.Fields(a.Command), " ")
	if cmd == "" {
		return ""
	}
	return strings.Join([]string{string(a.Kind), a.Repository, a.Branch, cmd}, "\x00")
}

// waitsOnlyOnDecisions reports whether a work identity's only open work is
// waiting for the operator's answers: it has an unanswered decision request,
// and no action, work in progress or blocker that does not wait on one of its
// requests.
func waitsOnlyOnDecisions(r Report, answered answeredFunc) bool {
	if r.Idle || len(r.OperatorActions) > 0 || len(r.InProgress) > 0 {
		return false
	}
	open := false
	for _, d := range r.DecisionsNeeded {
		if _, ok := answered(r.Identity, d.ID); !ok {
			open = true
		}
	}
	for _, bl := range r.Blocked {
		if bl.Decision == "" {
			return false
		}
	}
	return open
}

// SetUnreadable records the reports ReadReports could not decode. Their
// work identities did report, so they are no longer listed as missing.
func (b *Briefing) SetUnreadable(u []UnreadableReport) {
	b.Unreadable = append([]UnreadableReport{}, u...)
	b.Missing = slices.DeleteFunc(b.Missing, func(name string) bool {
		return slices.ContainsFunc(u, func(x UnreadableReport) bool { return x.Identity == name })
	})
}
