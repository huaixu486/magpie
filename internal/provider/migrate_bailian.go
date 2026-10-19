package provider

import (
	"slices"
	"strconv"
	"strings"
)

const (
	bailianPlanPreset     = "bailian-token-plan"
	bailianDecisionPreset = "bailian-decision"
	bailianPlanBase       = "https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1"
)

// SplitBailianTokenPlan moves saved uses of the Token Plan's decision
// model to a dedicated provider before agents or the gateway read them.
// store also applies the same conversion to edits, imports and restores.
func SplitBailianTokenPlan() (bool, error) {
	f, err := read()
	if err != nil {
		return false, err
	}
	if !splitBailianTokenPlans(&f) {
		return false, nil
	}
	if err := store(f); err != nil {
		return false, err
	}
	return true, nil
}

func splitBailianTokenPlans(f *file) bool {
	changed := false
	for i := 0; i < len(f.Providers); i++ {
		p := f.Providers[i]
		if !bailianPlanProvider(p) {
			continue
		}
		from := p.ID + "/" + BailianDecision
		used := slices.Contains(p.Models, BailianDecision)
		visitBailianRefs(f, func(ref string) string {
			if _, ok := bailianDecisionRef(ref, from); ok {
				used = true
			}
			return ref
		})
		// Merely having the old preset's decision URL never says a team
		// key can use it. Only a saved pick or reference creates a judge.
		if used {
			judge := bailianPlanJudge(p)
			id := bailianJudgeID(f, p.ID, &judge)
			to := id + "/" + BailianDecision
			visitBailianRefs(f, func(ref string) string {
				if tail, ok := bailianDecisionRef(ref, from); ok {
					prefix := ""
					if strings.HasPrefix(ref, "magpie/") {
						prefix = "magpie/"
					}
					return prefix + to + tail
				}
				return ref
			})
			changed = true
		}
		// Appending a judge may move the backing array: reacquire p by
		// index rather than writing through a pointer into the old one.
		p = f.Providers[i]
		if p.Decide != "" {
			p.Decide, changed = "", true
		}
		if slices.Contains(p.Models, BailianDecision) {
			p.Models = slices.DeleteFunc(slices.Clone(p.Models), func(m string) bool { return m == BailianDecision })
			p.NoModels = len(p.Models) == 0
			changed = true
		}
		if slices.Contains(p.PickedFrom, BailianDecision) {
			p.PickedFrom = slices.DeleteFunc(slices.Clone(p.PickedFrom), func(m string) bool { return m == BailianDecision })
			changed = true
		}
		f.Providers[i] = p
	}
	return changed
}

func bailianPlanProvider(p Provider) bool {
	return p.Preset == bailianPlanPreset &&
		(p.Chat == "" || strings.TrimRight(p.Chat, "/") == bailianPlanBase) &&
		(p.Decide == "" || strings.TrimRight(p.Decide, "/") == bailianPlanBase) &&
		(p.Chat != "" || p.Decide != "")
}

// bailianPlanJudge carries the same keys and request limits; a key's own
// proxy, model restrictions and on/off state keep applying after the move.
func bailianPlanJudge(p Provider) Provider {
	return Provider{
		Preset: bailianDecisionPreset, Icon: "bailian-color", Decide: bailianPlanBase, BaseAPI: "decide",
		Models: []string{BailianDecision}, Off: p.Off, NoModels: p.NoModels, Unlisted: p.Unlisted,
		Key: p.Key, KeyName: p.KeyName, KeyProtocol: p.KeyProtocol, KeyWeight: p.KeyWeight, Keys: slices.Clone(p.Keys),
		Routing: p.Routing, Sink: p.Sink, Affinity: p.Affinity,
		Headers: p.Headers, Proxy: p.Proxy, AccountProxies: p.AccountProxies, AccountModels: p.AccountModels,
		MaxConcurrency: p.MaxConcurrency, AccountConcurrency: p.AccountConcurrency,
		QueueLimit: p.QueueLimit, QueueWait: p.QueueWait, MaxRPM: p.MaxRPM,
		QuotaRest: p.QuotaRest, QuotaResetAt: p.QuotaResetAt, RestCap: p.RestCap,
		Website: Preset(bailianDecisionPreset).Website, KeysURL: Preset(bailianPlanPreset).KeysURL,
	}
}

// An existing judge is reused only when its endpoint, keys and settings
// agree. A workspace key and a Token Plan key must never be pooled.
func bailianJudgeID(f *file, source string, judge *Provider) string {
	taken, names := map[string]bool{}, map[string]bool{}
	for _, p := range f.Providers {
		taken[p.ID], names[strings.ToLower(p.Name)] = true, true
		for _, id := range p.Was {
			taken[id] = true
		}
		probe := *judge
		probe.ID, probe.Name = p.ID, p.Name
		if sameStored(normalize(probe), normalize(p)) {
			return p.ID
		}
	}
	judge.ID, judge.Name = bailianDecisionPreset, Preset(bailianDecisionPreset).Name
	for n := 2; taken[judge.ID]; n++ {
		judge.ID = bailianDecisionPreset + "-" + strconv.Itoa(n)
	}
	base := judge.Name
	for n := 2; names[strings.ToLower(judge.Name)]; n++ {
		judge.Name = base + " " + strconv.Itoa(n)
	}
	f.Providers = append(f.Providers, *judge)
	if at := slices.Index(f.Order, source); at >= 0 && !slices.Contains(f.Order, judge.ID) {
		f.Order = slices.Insert(f.Order, at+1, judge.ID)
	}
	return judge.ID
}

func bailianDecisionRef(ref, from string) (string, bool) {
	ref = strings.TrimPrefix(ref, "magpie/")
	if ref == from {
		return "", true
	}
	tail, ok := strings.CutPrefix(ref, from+":")
	return ":" + tail, ok
}

// Saved references move together in one providers.json write. Patterns
// stay as written; decision groups already require named members.
func visitBailianRefs(f *file, to func(string) string) {
	move := func(ids []string) []string {
		out := slices.Clone(ids)
		for i, id := range out {
			out[i] = to(id)
		}
		return out
	}
	for i := range f.Providers {
		f.Providers[i].Fallback = move(f.Providers[i].Fallback)
	}
	for i := range f.Groups {
		g := &f.Groups[i]
		g.Members, g.Off, g.Fast = move(g.Members), move(g.Off), move(g.Fast)
		g.Pick, g.Classifier = to(g.Pick), to(g.Classifier)
		g.Rules = slices.Clone(g.Rules)
		for j := range g.Rules {
			g.Rules[j].Use = to(g.Rules[j].Use)
		}
		g.Weights = renamedWeights(g.Weights, to)
	}
}
