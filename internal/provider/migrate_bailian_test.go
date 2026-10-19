package provider

import (
	"bytes"
	"os"
	"reflect"
	"slices"
	"testing"

	"github.com/yetone/magpie/internal/catalog"
)

func bailianSplitHome(t *testing.T) {
	t.Helper()
	isolate(t)
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("USERPROFILE", h)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
}

func oldBailianPlan() Provider {
	return Provider{
		ID: bailianPlanPreset, Name: "My plan", Preset: bailianPlanPreset, Key: "sk-sp-fixture",
		Chat: bailianPlanBase, Responses: bailianPlanBase,
		Anthropic:  "https://token-plan.cn-beijing.maas.aliyuncs.com/apps/anthropic",
		Models:     []string{"qwen3.8-max", BailianDecision},
		PickedFrom: []string{"qwen3.8-max", BailianDecision},
	}
}

// The reporter's missing decide field, the former mixed preset, and a
// renamed preset all move their saved classifier onto System One.
func TestSplitBailianTokenPlan(t *testing.T) {
	for _, tc := range []struct {
		name, id, decide string
	}{
		{"missing decision base", bailianPlanPreset, ""},
		{"old mixed preset", bailianPlanPreset, bailianPlanBase},
		{"renamed provider", "my-plan", bailianPlanBase},
		{"classifier only", bailianPlanPreset, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bailianSplitHome(t)
			p := oldBailianPlan()
			p.ID, p.Decide = tc.id, tc.decide
			if tc.name == "classifier only" {
				p.Models = []string{"qwen3.8-max"}
			}
			from, to := p.ID+"/"+BailianDecision, bailianDecisionPreset+"/"+BailianDecision
			key := KeyID(p.Key)
			p.Off, p.Routing, p.Proxy = true, Ordered, "http://127.0.0.1:7897"
			p.Keys = []KeyAccount{{Key: "sk-sp-second", Name: "Second", Off: true}}
			p.AccountProxies = map[string]string{key: "direct"}
			p.AccountModels = map[string][]string{key: {BailianDecision}}
			p.MaxConcurrency, p.QueueWait, p.MaxRPM = new(2), 5, 10
			f := file{
				Providers: []Provider{p, {ID: "relay", Name: "Relay", Chat: "https://relay.invalid/v1",
					Key: "k", Models: []string{"m"}, Fallback: []string{"magpie/" + from}}},
				Order: []string{p.ID, "relay"},
				Groups: []Group{
					{ID: "judges", Name: "Judges", Members: []string{from}, Pick: from,
						Off: []string{from}, Weights: map[string]int{from: 3}},
					{ID: "work", Name: "Work", Members: []string{"relay/m"},
						Classifier: from, Effort: EffortAuto, Rules: []Rule{{Use: "relay/m", Intent: "coding"}}},
				},
			}
			// Save would normalize new data. This is the actual old file.
			writeFile(t, Path(), f)
			changed, err := SplitBailianTokenPlan()
			if err != nil || !changed {
				t.Fatalf("split: %v %v", changed, err)
			}
			saved, err := read()
			if err != nil {
				t.Fatal(err)
			}
			plan := saved.Providers[0]
			if plan.Decide != "" || !slices.Equal(plan.Models, []string{"qwen3.8-max"}) ||
				!slices.Equal(plan.PickedFrom, []string{"qwen3.8-max"}) ||
				plan.Key != p.Key || plan.Chat != p.Chat || plan.Responses != p.Responses || plan.Anthropic != p.Anthropic {
				t.Fatalf("conversation provider: %+v", plan)
			}
			if len(saved.Providers) != 3 {
				t.Fatalf("providers: %d", len(saved.Providers))
			}
			judge := saved.Providers[2]
			if judge.ID != bailianDecisionPreset || !judge.DecideOnly() || judge.Decide != bailianPlanBase ||
				judge.Key != p.Key || !judge.Off || judge.Routing != p.Routing || judge.Proxy != p.Proxy ||
				!reflect.DeepEqual(judge.Keys, p.Keys) || !reflect.DeepEqual(judge.AccountModels, p.AccountModels) ||
				!reflect.DeepEqual(judge.AccountProxies, p.AccountProxies) ||
				judge.MaxConcurrency == nil || *judge.MaxConcurrency != 2 || judge.QueueWait != 5 || judge.MaxRPM != 10 {
				t.Fatalf("decision provider lost settings: %+v", judge)
			}
			if !slices.Equal(saved.Order, []string{p.ID, judge.ID, "relay"}) ||
				!slices.Equal(saved.Providers[1].Fallback, []string{"magpie/" + to}) {
				t.Fatalf("order or fallback: %+v", saved)
			}
			g := saved.Groups[0]
			if !slices.Equal(g.Members, []string{to}) || g.Pick != to ||
				!slices.Equal(g.Off, []string{to}) || g.Weights[to] != 3 ||
				saved.Groups[1].Classifier != to || saved.Groups[1].Rules[0].Use != "relay/m" {
				t.Fatalf("group references: %+v", saved.Groups)
			}
			before, _ := os.ReadFile(Path())
			if changed, err := SplitBailianTokenPlan(); err != nil || changed {
				t.Fatalf("second start: %v %v", changed, err)
			}
			after, _ := os.ReadFile(Path())
			if !bytes.Equal(before, after) {
				t.Fatal("second start rewrote the file")
			}
		})
	}
}

func TestBailianSplitDoesNotPoolWorkspaceKeys(t *testing.T) {
	bailianSplitHome(t)
	p := oldBailianPlan()
	existing, _ := FromPreset(bailianDecisionPreset)
	existing.Name, existing.Key = "My workspace", "workspace-key"
	existing.Decide = "https://ws-1.cn-beijing.maas.aliyuncs.com/compatible-mode/v1"
	writeFile(t, Path(), file{Providers: []Provider{p, existing}})
	if _, err := SplitBailianTokenPlan(); err != nil {
		t.Fatal(err)
	}
	f, _ := read()
	if len(f.Providers) != 3 || !sameStored(f.Providers[1], existing) ||
		f.Providers[2].ID != "bailian-decision-2" || f.Providers[2].Key != p.Key ||
		f.Providers[2].Decide != bailianPlanBase {
		t.Fatalf("workspace key was mixed with the plan: %+v", f.Providers)
	}
}

func TestBailianSplitOnlyWhenUsed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		p      Provider
		judges int
	}{
		{"team plan", Provider{ID: bailianPlanPreset, Preset: bailianPlanPreset, Key: "team",
			Chat: bailianPlanBase, Decide: bailianPlanBase}, 0},
		{"new plan", Provider{ID: bailianPlanPreset, Preset: bailianPlanPreset, Key: "team", Chat: bailianPlanBase}, 0},
		{"custom address", Provider{ID: bailianPlanPreset, Preset: bailianPlanPreset, Key: "custom",
			Chat: "https://relay.invalid/v1", Decide: "https://relay.invalid/v1", Models: []string{BailianDecision}}, 0},
		{"decision only pick", Provider{ID: bailianPlanPreset, Preset: bailianPlanPreset, Key: "personal",
			Chat: bailianPlanBase, Models: []string{BailianDecision}}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bailianSplitHome(t)
			writeFile(t, Path(), file{Providers: []Provider{tc.p}})
			if _, err := SplitBailianTokenPlan(); err != nil {
				t.Fatal(err)
			}
			f, _ := read()
			if len(f.Providers) != 1+tc.judges {
				t.Fatalf("created %d judges, want %d", len(f.Providers)-1, tc.judges)
			}
			if tc.name == "decision only pick" && (!f.Providers[0].NoModels || len(f.Providers[0].Models) != 0) {
				t.Fatal("an old decision-only pick now exposes chat models")
			}
			if tc.name == "custom address" && !sameStored(f.Providers[0], tc.p) {
				t.Fatal("custom provider was changed")
			}
		})
	}
}

func TestBailianSplitOnRestoreAndCachedList(t *testing.T) {
	bailianSplitHome(t)
	p := oldBailianPlan()
	from := p.ID + "/" + BailianDecision
	if err := Mirror([]Provider{p}, []Group{{ID: "work", Name: "Work", Classifier: from}}); err != nil {
		t.Fatal(err)
	}
	f, _ := read()
	if len(f.Providers) != 2 || f.Groups[0].Classifier != bailianDecisionPreset+"/"+BailianDecision {
		t.Fatalf("restored old references: %+v", f)
	}
	// The old vendor list can still hold the decision model after its
	// explicit pick was moved away.
	if err := catalog.SaveLive(p.ID, p.Chat, []catalog.Model{{ID: "qwen3.8-max"}, {ID: BailianDecision}}); err != nil {
		t.Fatal(err)
	}
	plan, err := Find(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range plan.Available() {
		if m.ID == BailianDecision {
			t.Fatal("cached list still offers the decision model as chat")
		}
	}
}
