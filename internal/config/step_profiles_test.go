package config

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/BertramPetersen/gatehouse/internal/types"
)

func TestStepProfilesCommandGatesAndRemovedNames(t *testing.T) {
	g := writeGlobalConfig(t, "agent: codex\nagent_profiles: {economical: {codex: {model: small}}}\n")
	c := Merge(g, &RepoConfig{Gates: []Gate{{Name: "shell", After: types.StepTest, Command: "true"}, {Name: "agent", After: types.StepTest, Instructions: "Check tests"}}})
	p, missing, err := c.ResolveStepProfiles(map[string]string{"gate.test.agent": "removed"})
	if err != nil || len(missing) != 1 || missing[0] != "gate.test.agent" {
		t.Fatalf("missing: %v %v", missing, err)
	}
	if p.Steps["gate.test.shell"].Name != "default" {
		t.Fatal("command gate prompted")
	}
	p.Runners = []types.AgentName{types.AgentCodex}
	data, _ := json.Marshal(p)
	if _, err := ParseStepProfilePlan(string(data)); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{string(data) + "{}", `{"version":999}`, strings.Replace(string(data), `"version":1`, `"unknown":true,"version":1`, 1)} {
		if _, err := ParseStepProfilePlan(invalid); err == nil {
			t.Fatal("invalid pin accepted")
		}
	}
}

func TestStepProfilesResolveAndRequireLocalGateChoices(t *testing.T) {
	g := writeGlobalConfig(t, `agent: codex
agent_config:
  codex: {model: base, effort: medium}
agent_profiles:
  thorough:
    codex: {model: large, effort: high}
  economical:
    codex: {effort: low}
agent_step_profiles:
  review: thorough
  test: economical
`)
	c := Merge(g, &RepoConfig{Gates: []Gate{{Name: "architecture", After: types.StepTest, Instructions: "Check imports"}}})
	plan, missing, err := c.ResolveStepProfiles(nil)
	if err != nil || len(missing) != 1 {
		t.Fatalf("missing = %v, err = %v", missing, err)
	}
	if p := plan.Steps[types.StepReview].Agents["codex"]; p.Model != "large" || p.Effort != "high" {
		t.Fatalf("review: %+v", p)
	}
	if p := plan.Steps[types.StepTest].Agents["codex"]; p.Model != "base" || p.Effort != "low" {
		t.Fatalf("test: %+v", p)
	}
	plan, missing, err = c.ResolveStepProfiles(map[string]string{"gate.test.architecture": "economical"})
	if err != nil || len(missing) != 0 {
		t.Fatalf("resolve: %v %v", missing, err)
	}
	if plan.Steps["gate.test.architecture"].Name != "economical" {
		t.Fatal("gate choice not applied")
	}
}

func TestStepProfilesLegacyAndRepoIsolation(t *testing.T) {
	repo, err := LoadRepoFromBytes([]byte(`agent_profiles: {hostile: {codex: {model: hostile}}}
agent_step_profiles: {review: hostile}
gates: [{name: legacy, after: test, instructions: Check imports}]
`))
	if err != nil {
		t.Fatal(err)
	}
	c := Merge(DefaultGlobalConfig(), repo)
	plan, missing, err := c.ResolveStepProfiles(nil)
	if err != nil || len(missing) != 0 || plan != nil {
		t.Fatalf("legacy: %+v %v %v", plan, missing, err)
	}
}

func TestStepProfilesRejectInvalidConfiguration(t *testing.T) {
	for _, input := range []string{
		"agent_profiles: {default: {codex: {model: x}}}",
		"agent_profiles: {small: {codex: {temperature: 1}}}",
		"agent_profiles: {small: {rovodev: {model: x}}}",
		"agent_step_profiles: {review: missing}",
		"agent_step_profiles: {push: default}",
		"agent_step_profiles: {gate.test.x: default}",
	} {
		t.Run(input, func(t *testing.T) {
			if _, err := LoadGlobalFromBytes([]byte(input)); err == nil {
				t.Fatal("accepted invalid config")
			}
		})
	}
}
