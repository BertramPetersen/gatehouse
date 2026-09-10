package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/BertramPetersen/gatehouse/internal/agent"
	"github.com/BertramPetersen/gatehouse/internal/config"
	"github.com/BertramPetersen/gatehouse/internal/runenv"
	"github.com/BertramPetersen/gatehouse/internal/types"
)

// stepAgents exposes the review chain's session capabilities and owns every
// cached chain. Steps receive borrowed agents; the run closes their owner once.
type stepAgents struct {
	agent.Agent
	byStep map[types.StepName]agent.Agent
	owned  []agent.Agent
}

func (a *stepAgents) ForStep(step types.StepName) agent.Agent { return a.byStep[step] }
func (a *stepAgents) SupportsSessionResume() bool             { return agent.SupportsSessionResume(a.Agent) }
func (a *stepAgents) SupportsSessionProvider(provider string) bool {
	return agent.SupportsSessionProvider(a.Agent, provider)
}
func (a *stepAgents) ReportsAgentAttempts() bool { return agent.ReportsAgentAttempts(a.Agent) }
func (a *stepAgents) NeutralizesGateInstructions() bool {
	return agent.NeutralizesGateInstructions(a.Agent)
}
func (a *stepAgents) Close() error {
	var errs []error
	for _, ag := range a.owned {
		errs = append(errs, ag.Close())
	}
	return errors.Join(errs...)
}

func newPipelineAgent(ctx context.Context, cfg *config.Config, evidenceRoot string, lookPath func(string) (string, error), environment runenv.Overlay) (agent.Agent, error) {
	if cfg.StepProfiles == nil {
		return newProfileAgent(ctx, cfg, evidenceRoot, lookPath, environment)
	}
	pin := cfg.StepProfiles
	if len(pin.Missing()) != 0 {
		return nil, fmt.Errorf("custom gate model configuration required")
	}
	if cfg.AgentArgsDigest() != pin.RawArgsDigest {
		return nil, fmt.Errorf("agent_args_override changed since this run pinned its models; restore the original overrides or start a new run")
	}
	cfg.Agents = append([]types.AgentName(nil), pin.Runners...)
	cfg.Agent = pin.Runners[0]
	if err := cfg.ResolveAgent(ctx, lookPath); err != nil {
		return nil, err
	}
	if !agentListsEqual(cfg.Agents, pin.Runners) {
		return nil, fmt.Errorf("a pinned agent is no longer available; restore it or start a new run")
	}
	owner := &stepAgents{byStep: map[types.StepName]agent.Agent{}}
	cache := map[string]agent.Agent{}
	for step, profile := range pin.Steps {
		key, _ := json.Marshal(profile.Agents)
		ag := cache[string(key)]
		if ag == nil {
			c := *cfg
			c.AgentConfig = profile.Agents
			var err error
			ag, err = newProfileAgent(ctx, &c, evidenceRoot, lookPath, environment)
			if err != nil {
				owner.Close()
				return nil, fmt.Errorf("profile %s for %s: %w", profile.Name, step, err)
			}
			cache[string(key)] = ag
			owner.owned = append(owner.owned, ag)
		}
		owner.byStep[step] = ag
	}
	owner.Agent = owner.byStep[types.StepReview]
	return owner, nil
}
