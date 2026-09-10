package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/BertramPetersen/gatehouse/internal/agent"
	"github.com/BertramPetersen/gatehouse/internal/agentcfg"
	"github.com/BertramPetersen/gatehouse/internal/config"
	"github.com/BertramPetersen/gatehouse/internal/pipeline"
	"github.com/BertramPetersen/gatehouse/internal/runenv"
	"github.com/BertramPetersen/gatehouse/internal/types"
)

func TestStepAgentFactoryPassesDistinctNativeArguments(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX argument recorder")
	}
	for _, rawPin := range []bool{false, true} {
		t.Run(fmt.Sprint(rawPin), func(t *testing.T) {
			dir := t.TempDir()
			spy := filepath.Join(dir, "claude")
			script := `#!/bin/sh
printf '%s\n' "$@" > "$MODEL_ARGS_FILE"
printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"structured_output":{"findings":[],"summary":"clean"}}'
`
			if err := os.WriteFile(spy, []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			c := &config.Config{Agent: types.AgentClaude, Agents: []types.AgentName{types.AgentClaude}, AgentPathOverride: map[string]string{"claude": spy},
				AgentConfig:       map[string]agentcfg.Profile{"claude": {Model: "small", Effort: agentcfg.EffortLow}},
				AgentProfiles:     map[string]map[string]agentcfg.Profile{"thorough": {"claude": {Model: "large", Effort: agentcfg.EffortHigh}}},
				AgentStepProfiles: map[types.StepName]string{types.StepReview: "thorough"}}
			if rawPin {
				c.AgentArgsOverride = map[string][]string{"claude": {"--model", "native"}}
			}
			c.StepProfiles, _, _ = c.ResolveStepProfiles(nil)
			// Later edits must not retarget an existing run's resolved settings.
			c.AgentProfiles["thorough"]["claude"] = agentcfg.Profile{Model: "changed"}
			a, err := newPipelineAgent(context.Background(), c, dir, fakeLookPath, runenv.Overlay{})
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			for _, step := range []types.StepName{types.StepReview, types.StepTest} {
				capture := filepath.Join(dir, string(step)+".args")
				_, err := pipeline.AgentForStep(a, step).Run(context.Background(), agent.RunOpts{CWD: dir, Prompt: "Check the change", Env: []string{"MODEL_ARGS_FILE=" + capture}})
				if err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(capture)
				if err != nil {
					t.Fatal(err)
				}
				model, effort := "small", "low"
				if step == types.StepReview {
					model, effort = "large", "high"
				}
				if rawPin {
					model = "native"
				}
				if strings.Count(string(data), "--model\n") != 1 || !strings.Contains(string(data), "--model\n"+model+"\n") || !strings.Contains(string(data), "--effort\n"+effort+"\n") {
					t.Fatalf("%s received wrong native arguments: %s", step, data)
				}
			}
		})
	}
}

func TestStepAgentFactorySeparatesProfilesAndSharesEqualChains(t *testing.T) {
	c := &config.Config{Agent: types.AgentCodex, Agents: []types.AgentName{types.AgentCodex}, AgentProfiles: map[string]map[string]agentcfg.Profile{
		"thorough": {"codex": {Model: "large", Effort: agentcfg.EffortHigh}},
	}, AgentStepProfiles: map[types.StepName]string{types.StepReview: "thorough"}}
	var err error
	c.StepProfiles, _, err = c.ResolveStepProfiles(nil)
	if err != nil {
		t.Fatal(err)
	}
	a, err := newPipelineAgent(context.Background(), c, t.TempDir(), fakeLookPath, runenv.Overlay{})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	review := pipeline.AgentForStep(a, types.StepReview)
	test := pipeline.AgentForStep(a, types.StepTest)
	if review == test {
		t.Fatal("review and test share agents despite different model pins")
	}
	if test != pipeline.AgentForStep(a, types.StepDocument) {
		t.Fatal("equal profiles did not share chain")
	}
	if agent.SupportsSessionResume(a) != agent.SupportsSessionResume(review) {
		t.Fatal("sessions must use review chain")
	}
	c.AgentArgsOverride = map[string][]string{"codex": {"-m", "changed"}}
	if a, err := newPipelineAgent(context.Background(), c, t.TempDir(), fakeLookPath, runenv.Overlay{}); err == nil {
		a.Close()
		t.Fatal("changed native pins bypassed run model pin")
	}
}
