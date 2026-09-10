//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BertramPetersen/gatehouse/internal/types"
)

func runModelProfilesJourney(t *testing.T) {
	h := NewHarness(t, SetupOpts{Agent: "claude", Scenario: cleanReviewScenario(t)})
	path := filepath.Join(h.NMHome, "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, []byte(`agent_profiles:
  thorough:
    claude: {model: large, effort: high}
  economical:
    claude: {model: small, effort: low}
agent_step_profiles:
  review: thorough
  test: economical
`)...)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	pushMainRepoConfig(t, h, `gates:
  - name: arch-fitness
    after: test
    instructions: Check package boundaries.
  - name: mutation-budget
    after: test
    instructions: Check test coverage.
  - name: shell-check
    after: test
    command: "true"
`)
	if out, err := h.Run("init"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	const branch = "feature/model-profiles"
	h.CommitChange(branch, "feature.txt", "model routing\n", "exercise model routing")
	fw := h.AddWorktree(branch)
	out, err := h.RunInDir(fw, "axi", "run", "--yes", "--intent", "Exercise step-specific local model choices")
	if err != nil || !strings.Contains(out, "model-configuration-required") {
		t.Fatalf("setup: %v\n%s", err, out)
	}
	run := h.ActiveRun(branch)
	if run == nil || run.Status != types.RunPending || len(run.ModelSetup) != 2 || len(run.Steps) != 0 {
		t.Fatalf("setup snapshot: %+v", run)
	}
	if calls := h.AgentInvocations(); len(calls) != 0 {
		t.Fatalf("agents ran before choices: %+v", calls)
	}
	out, err = h.RunInDir(fw, "axi", "models", "--run", run.ID)
	for _, want := range []string{"economical", "thorough", "gate.test.arch-fitness", "gate.test.mutation-budget", "model=small"} {
		if err != nil || !strings.Contains(out, want) {
			t.Fatalf("models missing %q: %v\n%s", want, err, out)
		}
	}
	if out, err := h.RunInDir(fw, "axi", "models", "--run", run.ID, "--set", "gate.test.arch-fitness=economical"); err == nil {
		t.Fatalf("partial setup accepted: %s", out)
	}
	if out, err := h.Run("daemon", "restart", "--force"); err != nil {
		t.Fatalf("restart: %v\n%s", err, out)
	}
	if resumed := h.RunInfo(run.ID); resumed.Status != types.RunPending || len(resumed.ModelSetup) != 2 {
		t.Fatalf("lost setup on restart: %+v", resumed)
	}
	out, err = h.RunInDir(fw, "axi", "models", "--run", run.ID,
		"--set", "gate.test.arch-fitness=economical", "--set", "gate.test.mutation-budget=economical")
	if err != nil || !strings.Contains(out, "models-configured") {
		t.Fatalf("save: %v\n%s", err, out)
	}
	finished := h.WaitForRun(branch, 90*time.Second)
	if finished.ID != run.ID || finished.Status != types.RunCompleted {
		t.Fatalf("same run did not finish: %+v", finished)
	}
	review, economical := false, 0
	for _, call := range h.AgentInvocations() {
		args := strings.Join(call.Args, "\n")
		if strings.Contains(call.Prompt, reviewStepPromptMarker) {
			review = strings.Contains(args, "--model\nlarge") && strings.Contains(args, "--effort\nhigh")
		}
		if strings.Contains(call.Prompt, `validation gate named "arch-fitness"`) || strings.Contains(call.Prompt, `validation gate named "mutation-budget"`) {
			if !strings.Contains(args, "--model\nsmall") || !strings.Contains(args, "--effort\nlow") {
				t.Fatalf("gate used wrong profile: %v", call.Args)
			}
			economical++
		}
	}
	if !review || economical != 2 {
		t.Fatalf("native routing: review=%v gates=%d", review, economical)
	}
	out, err = h.RunInDir(fw, "axi", "models", "--run", run.ID, "--forget", "gate.test.arch-fitness")
	if err != nil || !strings.Contains(out, "models-forgotten") {
		t.Fatalf("forget: %v\n%s", err, out)
	}
}
