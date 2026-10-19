package provider

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
)

// Alibaba Cloud's Token Plan (Jeremy on Discord): its own host, apart from
// DashScope's pay as you go, with chat completions and Anthropic messages.
func TestQwenTokenPlan(t *testing.T) {
	p, err := FromPreset("qwen-token-plan")
	if err != nil {
		t.Fatal(err)
	}
	if p.Chat != "https://token-plan.maas.qianwenaiapi.com/compatible-mode/v1" || p.Anthropic != "https://token-plan.maas.qianwenaiapi.com/apps/anthropic" || p.Responses != "" {
		t.Fatalf("endpoints: %q %q %q", p.Chat, p.Responses, p.Anthropic)
	}
	if got := p.planModels(nil); len(got) != len(Preset("qwen-token-plan").Models) || got[1].ID != "qwen3.8-max" {
		t.Fatalf("plan's: %+v", got)
	}
	im, _ := imported("Qwen", "sk-sp-x", endpoints{chat: "https://token-plan.maas.qianwenaiapi.com/compatible-mode/v1"}, nil)
	if im.Preset != "qwen-token-plan" {
		t.Fatalf("imported: %+v", im)
	}
}

// Bailian's Token Plan (#1506, IamMiao), the same plan as the Qwen AI
// platform's but sold on Alibaba Cloud, at its own host: chat completions
// and Anthropic messages as token-plan-team-quickstart gives them. Its
// decision model is Bailian Decision Model's Token Plan region alone, so
// a team plan never lists a model it can't answer with.
func TestBailianTokenPlan(t *testing.T) {
	isolate(t)
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("USERPROFILE", h)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(h, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(h, ".cache"))
	p, err := FromPreset("bailian-token-plan")
	if err != nil {
		t.Fatal(err)
	}
	const root = "https://token-plan.cn-beijing.maas.aliyuncs.com"
	if p.Chat != root+"/compatible-mode/v1" || p.Anthropic != root+"/apps/anthropic" || p.Responses != "" || p.Decide != "" {
		t.Fatalf("endpoints: %+v", p)
	}
	if pr := Preset("bailian-token-plan"); pr.Icon != "bailian-color" || !slices.Equal(pr.Models, Preset("qwen-token-plan").Models) {
		t.Fatalf("preset: %+v", pr)
	}
	if got := p.planModels(nil); len(got) != len(alibabaPlanModels) || got[1].ID != "qwen3.8-max" {
		t.Fatalf("plan's: %+v", got)
	}
	im, _ := imported("Bailian", "sk-sp-x", endpoints{chat: root + "/compatible-mode/v1"}, nil)
	if im.Preset != "bailian-token-plan" {
		t.Fatalf("imported: %+v", im)
	}

	p.Key = "sk-sp-1"
	id, err := Add(p)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := Find(id)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Decides() || saved.DecideOnly() || saved.DecidesModel(BailianDecision) {
		t.Fatalf("the Token Plan serves decisions: %+v", saved)
	}
	if ms := saved.DecisionModels(); len(ms) != 0 {
		t.Fatalf("decision models: %+v", ms)
	}
}

// The decision model on the Qwen AI platform's pay as you go (#1506), at
// maas.qianwenaiapi.com as decision-model-preview's page gives it, apart
// from Bailian's, which shows Bailian's icon.
func TestQwenDecisionPreset(t *testing.T) {
	p, err := FromPreset("qwen-decision")
	if err != nil {
		t.Fatal(err)
	}
	if !p.DecideOnly() || p.Jev() != BailianDecision || p.DecideVia() != ViaSystemOne {
		t.Fatalf("preset: %+v", p)
	}
	if u, err := p.DecideModelURL(context.Background(), BailianDecision); err != nil || u != "https://maas.qianwenaiapi.com/compatible-mode/v1/systemone" {
		t.Fatalf("asked at %q %v", u, err)
	}
	if Preset("qwen-decision").Icon != "qwen-color" || Preset("bailian-decision").Icon != "bailian-color" {
		t.Fatal("icons")
	}
}
