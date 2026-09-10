package pipeline

import (
	"context"
	"testing"

	"github.com/BertramPetersen/gatehouse/internal/agent"
	"github.com/BertramPetersen/gatehouse/internal/config"
	"github.com/BertramPetersen/gatehouse/internal/types"
)

type routedTestAgent struct {
	agent.Agent
	agents map[types.StepName]agent.Agent
}

func (a routedTestAgent) ForStep(step types.StepName) agent.Agent { return a.agents[step] }

func TestExecutorRoutesAndRecordsStepProfiles(t *testing.T) {
	database, paths, run, repo := setupTest(t)
	large := &fallbackUsageAgent{name: "large", result: &agent.Result{Model: "large"}}
	small := &fallbackUsageAgent{name: "small", result: &agent.Result{Model: "small"}}
	routed := routedTestAgent{Agent: large, agents: map[types.StepName]agent.Agent{types.StepReview: large, types.StepTest: small, "gate.test.custom": small}}
	var steps []Step
	cfg := &config.Config{StepProfiles: &config.StepProfilePlan{Steps: map[types.StepName]config.StepProfile{}}}
	for _, name := range []types.StepName{types.StepReview, types.StepTest, "gate.test.custom"} {
		profile := "economical"
		if name == types.StepReview {
			profile = "thorough"
		}
		cfg.StepProfiles.Steps[name] = config.StepProfile{Name: profile}
		steps = append(steps, &adaptiveCallStep{name: name, fn: func(sctx *StepContext) (*StepOutcome, error) {
			_, err := sctx.Agent.Run(sctx.Ctx, agent.RunOpts{Prompt: "Check change"})
			return &StepOutcome{}, err
		}})
	}
	executor := NewExecutor(database, paths, cfg, routed, steps, nil)
	if err := executor.Execute(context.Background(), run, repo, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	rows, err := database.GetAgentInvocationsByRun(run.ID)
	if err != nil || len(rows) != 3 {
		t.Fatalf("rows: %v %v", rows, err)
	}
	for _, row := range rows {
		model, profile := "small", "economical"
		if row.StepName == "review" {
			model, profile = "large", "thorough"
		}
		if row.Model != model || row.Profile != profile {
			t.Fatalf("wrong routing: %+v", row)
		}
	}
}
