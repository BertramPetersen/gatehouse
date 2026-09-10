package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/BertramPetersen/gatehouse/internal/agentcfg"
	"github.com/BertramPetersen/gatehouse/internal/types"
)

// StepProfilePlan is a run's immutable model policy. Empty gate names represent
// unresolved operator choices; no agent may launch until those are filled.
// RawArgsDigest detects changes to native pins without storing arguments that
// could contain credentials. Recovery refuses a changed digest.
type StepProfilePlan struct {
	Version       int                            `json:"version"`
	Runners       []types.AgentName              `json:"runners"`
	RawArgsDigest string                         `json:"raw_args_digest"`
	Steps         map[types.StepName]StepProfile `json:"steps"`
	Skip          []types.StepName               `json:"skip,omitempty"`
}

type StepProfile struct {
	Name   string                      `json:"name"`
	Agents map[string]agentcfg.Profile `json:"agents"`
}

func parseStepProfiles(cfg *GlobalConfig, raw globalConfigRaw) error {
	cfg.AgentProfiles = make(map[string]map[string]agentcfg.Profile, len(raw.AgentProfiles))
	for name, entries := range raw.AgentProfiles {
		if name == "default" || !types.ValidCustomGateLabel(name) {
			return fmt.Errorf("invalid agent_profiles name %q (default is reserved)", name)
		}
		profiles, err := parseAgentConfig(entries)
		if err != nil {
			return fmt.Errorf("agent_profiles.%s: %w", name, err)
		}
		cfg.AgentProfiles[name] = profiles
	}
	for step, name := range raw.AgentStepProfiles {
		if !types.IsCoreStepName(step) || step == types.StepPush {
			return fmt.Errorf("agent_step_profiles: %q is not an agent-driven core step", step)
		}
		if name != "default" {
			if _, ok := cfg.AgentProfiles[name]; !ok {
				return fmt.Errorf("agent_step_profiles.%s: unknown profile %q", step, name)
			}
		}
	}
	cfg.AgentStepProfiles = raw.AgentStepProfiles
	return nil
}

func (c *Config) ProfileNames() []string {
	names := []string{"default"}
	for name := range c.AgentProfiles {
		names = append(names, name)
	}
	sort.Strings(names[1:])
	return names
}

func (c *Config) NamedProfile(name string) (StepProfile, error) {
	overlay, ok := c.AgentProfiles[name]
	if name != "default" && !ok {
		return StepProfile{}, fmt.Errorf("unknown local agent profile %q", name)
	}
	result := StepProfile{Name: name, Agents: make(map[string]agentcfg.Profile)}
	for agent, p := range c.AgentConfig {
		result.Agents[agent] = p
	}
	for agent, p := range overlay {
		base := result.Agents[agent]
		if p.Model != "" {
			base.Model = p.Model
		}
		if p.Effort != "" {
			base.Effort = p.Effort
		}
		result.Agents[agent] = base
	}
	return result, nil
}

// ResolveStepProfiles consumes only operator-owned choices. Repository gates
// supply identities, never profile names. Legacy configurations remain opt-out.
func (c *Config) ResolveStepProfiles(choices map[string]string) (*StepProfilePlan, []types.StepName, error) {
	if len(c.AgentProfiles) == 0 && len(c.AgentStepProfiles) == 0 {
		return nil, nil, nil
	}
	plan := &StepProfilePlan{Version: 1, Steps: map[types.StepName]StepProfile{}, Runners: append([]types.AgentName(nil), c.Agents...), RawArgsDigest: c.AgentArgsDigest()}
	if len(plan.Runners) == 0 {
		plan.Runners = []types.AgentName{c.Agent}
	}
	for _, step := range types.AllSteps() {
		name := c.AgentStepProfiles[step]
		if name == "" {
			name = "default"
		}
		p, err := c.NamedProfile(name)
		if err != nil {
			return nil, nil, err
		}
		plan.Steps[step] = p
	}
	for _, g := range c.Gates {
		name := "default"
		if g.IsAgent() {
			name = choices[string(g.StepName())]
		}
		// A removed profile needs a new choice on a future run.
		if _, ok := c.AgentProfiles[name]; name != "default" && !ok {
			name = ""
		}
		if name == "" {
			plan.Steps[g.StepName()] = StepProfile{}
			continue
		}
		p, err := c.NamedProfile(name)
		if err != nil {
			return nil, nil, err
		}
		plan.Steps[g.StepName()] = p
	}
	return plan, plan.Missing(), nil
}

func (p *StepProfilePlan) Missing() []types.StepName {
	var missing []types.StepName
	if p != nil {
		for step, profile := range p.Steps {
			if profile.Name == "" {
				missing = append(missing, step)
			}
		}
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i] < missing[j] })
	return missing
}

func (c *Config) AgentArgsDigest() string {
	data, _ := json.Marshal(c.AgentArgsOverride)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func ParseStepProfilePlan(data string) (*StepProfilePlan, error) {
	if data == "" {
		return nil, nil
	}
	var p StepProfilePlan
	dec := json.NewDecoder(strings.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("decode run model profiles: %w", err)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("trailing data in run model profiles")
	}
	if digest, err := hex.DecodeString(p.RawArgsDigest); err != nil || len(digest) != sha256.Size {
		return nil, fmt.Errorf("invalid native argument digest")
	}
	if p.Version != 1 || len(p.Runners) == 0 || len(p.Steps) == 0 || p.RawArgsDigest == "" {
		return nil, fmt.Errorf("invalid run model profile pin")
	}
	for _, name := range p.Runners {
		if !agentcfg.Known(name) {
			return nil, fmt.Errorf("invalid pinned agent %q", name)
		}
	}
	for _, step := range types.AllSteps() {
		if _, ok := p.Steps[step]; !ok {
			return nil, fmt.Errorf("missing pinned profile for %s", step)
		}
	}
	for step, profile := range p.Steps {
		if !types.IsCoreStepName(step) && !step.IsCustomGate() {
			return nil, fmt.Errorf("invalid pinned step %q", step)
		}
		if profile.Name == "" && !step.IsCustomGate() {
			return nil, fmt.Errorf("missing core profile %s", step)
		}
		if profile.Name != "" && !types.ValidCustomGateLabel(profile.Name) {
			return nil, fmt.Errorf("invalid pinned profile name")
		}
		if profile.Name == "" && len(profile.Agents) != 0 {
			return nil, fmt.Errorf("unresolved profile carries model settings")
		}
		for name, value := range profile.Agents {
			if err := agentcfg.Validate(types.AgentName(name), value); err != nil {
				return nil, err
			}
		}
	}
	for _, step := range p.Skip {
		if !types.IsCoreStepName(step) {
			return nil, fmt.Errorf("invalid skipped step in model pin")
		}
	}
	return &p, nil
}

func (c *Config) SameStepProfiles(a, b types.StepName) bool {
	if c.StepProfiles == nil {
		return true
	}
	left, _ := json.Marshal(c.StepProfiles.Steps[a].Agents)
	right, _ := json.Marshal(c.StepProfiles.Steps[b].Agents)
	return string(left) == string(right)
}
