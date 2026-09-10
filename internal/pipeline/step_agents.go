package pipeline

import (
	"github.com/BertramPetersen/gatehouse/internal/agent"
	"github.com/BertramPetersen/gatehouse/internal/types"
)

// AgentForStep lets the executor bind routing before adding its deadline,
// phase, lifecycle and evidence wrappers. Single-agent embeddings are unchanged.
func AgentForStep(a agent.Agent, step types.StepName) agent.Agent {
	if router, ok := a.(interface {
		ForStep(types.StepName) agent.Agent
	}); ok {
		return router.ForStep(step)
	}
	return a
}
