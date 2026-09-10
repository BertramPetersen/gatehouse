package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/BertramPetersen/gatehouse/internal/agent"
	"github.com/BertramPetersen/gatehouse/internal/config"
	"github.com/BertramPetersen/gatehouse/internal/db"
	"github.com/BertramPetersen/gatehouse/internal/forgecontext"
	"github.com/BertramPetersen/gatehouse/internal/git"
	"github.com/BertramPetersen/gatehouse/internal/ipc"
	"github.com/BertramPetersen/gatehouse/internal/pipeline/steps"
	"github.com/BertramPetersen/gatehouse/internal/types"
	"github.com/BertramPetersen/gatehouse/internal/worktrees"
)

func (m *RunManager) prepareRunModelPlan(ctx context.Context, run *db.Run, cfg *config.Config, skip []types.StepName) error {
	if steps.IsDemoMode() || (len(cfg.AgentProfiles) == 0 && len(cfg.AgentStepProfiles) == 0) {
		return nil
	}
	if err := cfg.ResolveAgent(ctx, exec.LookPath); err != nil {
		return err
	}
	choices, err := m.db.GetRepoModelChoices(run.RepoID)
	if err != nil {
		return err
	}
	plan, _, err := cfg.ResolveStepProfiles(choices)
	if err != nil {
		return err
	}
	plan.Skip = append([]types.StepName(nil), skip...)
	payload, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	if err := m.db.InsertRunModelPlan(run.ID, string(payload)); err != nil {
		return err
	}
	cfg.StepProfiles = plan
	return nil
}

func (m *RunManager) loadRunModelPlan(runID string, cfg *config.Config) error {
	data, err := m.db.GetRunModelPlan(runID)
	if err != nil {
		return err
	}
	cfg.StepProfiles, err = config.ParseStepProfilePlan(data)
	if err != nil {
		return err
	}
	if cfg.StepProfiles != nil {
		if len(cfg.StepProfiles.Steps) != len(types.AllSteps())+len(cfg.Gates) {
			return fmt.Errorf("model pin does not match pinned gates")
		}
		for _, gate := range cfg.Gates {
			if _, ok := cfg.StepProfiles.Steps[gate.StepName()]; !ok {
				return fmt.Errorf("model pin is missing gate %s", gate.StepName())
			}
		}
	}
	return nil
}

// A pending setup has no steps, but keeps its worktree and gate declaration.
// Recover it independently of approval recovery, which requires step results.
func (m *RunManager) preparePendingModelRun(ctx context.Context, run *db.Run) (*recoveredRunPlan, error) {
	data, err := m.db.GetRunModelPlan(run.ID)
	if err != nil {
		return nil, err
	}
	pin, err := config.ParseStepProfilePlan(data)
	if err != nil {
		return nil, err
	}
	if pin == nil {
		return nil, fmt.Errorf("pending run has no model setup")
	}
	gates, err := m.pinnedRunGates(run.ID)
	if err != nil {
		return nil, err
	}
	if len(pin.Steps) != len(types.AllSteps())+len(gates) {
		return nil, fmt.Errorf("model pin does not match pinned gates")
	}
	for _, gate := range gates {
		profile, ok := pin.Steps[gate.StepName()]
		if !ok || (!gate.IsAgent() && profile.Name != "default") {
			return nil, fmt.Errorf("invalid model pin for gate %s", gate.StepName())
		}
	}
	results, err := m.db.GetStepsByRun(run.ID)
	if err != nil {
		return nil, err
	}
	if len(results) != 0 {
		return nil, fmt.Errorf("model setup run already has step results")
	}
	repo, err := m.db.GetRepo(run.RepoID)
	if err != nil {
		return nil, err
	}
	if repo == nil {
		return nil, fmt.Errorf("model setup repository is missing")
	}
	workDir := worktrees.RecordedDir(m.paths, run.WorktreePath(), repo.ID, run.ID)
	if info, err := os.Stat(workDir); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("model setup worktree is missing")
	}
	if head, err := git.HeadSHA(ctx, workDir); err != nil || head != run.HeadSHA {
		return nil, fmt.Errorf("model setup worktree head changed")
	}
	common, err := git.Run(ctx, workDir, "rev-parse", "--git-common-dir")
	if err != nil {
		return nil, err
	}
	if !samePath(resolveGitPath(workDir, common), m.paths.RepoDir(repo.ID)) {
		return nil, fmt.Errorf("model setup worktree does not belong to its gate")
	}
	plan := &recoveredRunPlan{run: run, repo: repo, workDir: workDir, gateDir: m.paths.RepoDir(repo.ID), agent: agent.NewNoop(), configuring: len(pin.Missing()) > 0, fresh: true}
	if plan.configuring {
		return plan, nil
	}
	cfg, err := m.loadRecoveredConfig(ctx, run, repo, workDir)
	if err != nil {
		return nil, err
	}
	forge, err := forgecontext.Resolve(ctx, cfg.ForgeProfiles, repo.UpstreamURL, repo.ForkURL)
	if err != nil {
		return nil, err
	}
	ag, err := newPipelineAgent(ctx, cfg, m.paths.EvidenceRoot(cfg.Test.Evidence.LocalRoot), exec.LookPath, forgeEnvironment(forge))
	if err != nil {
		return nil, err
	}
	plan.cfg = cfg
	plan.forge = forge
	plan.agent = ag
	plan.steps = steps.WithCustomGates(m.steps(), cfg.Gates)
	return plan, nil
}

func modelPlanToken(data string, cfg *config.Config) string {
	catalog, _ := json.Marshal(struct {
		Profiles any
		Base     any
		Args     string
	}{cfg.AgentProfiles, cfg.AgentConfig, cfg.AgentArgsDigest()})
	sum := sha256.Sum256(append([]byte(data), catalog...))
	return hex.EncodeToString(sum[:])
}

func (m *RunManager) ModelSetup(runID string) (*ipc.ModelSetupResult, error) {
	run, err := m.db.GetRun(runID)
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, fmt.Errorf("run not found")
	}
	data, err := m.db.GetRunModelPlan(runID)
	if err != nil {
		return nil, err
	}
	pin, err := config.ParseStepProfilePlan(data)
	if err != nil {
		return nil, err
	}
	result := &ipc.ModelSetupResult{RunID: runID}
	result.Saved, err = m.db.GetRepoModelChoices(run.RepoID)
	if err != nil {
		return nil, err
	}
	if pin == nil || run.Status != types.RunPending {
		return result, nil
	}
	result.Missing = pin.Missing()
	global, err := config.LoadGlobal(m.paths.ConfigFile())
	if err != nil {
		return nil, err
	}
	cfg := config.Merge(global, &config.RepoConfig{})
	result.Token = modelPlanToken(data, cfg)
	for _, name := range cfg.ProfileNames() {
		p, err := cfg.NamedProfile(name)
		if err != nil {
			return nil, err
		}
		detail := ""
		for _, runner := range pin.Runners {
			settings := p.Agents[string(runner)].String()
			if settings == "" {
				settings = "harness defaults"
			}
			detail += fmt.Sprintf("%s: %s; ", runner, settings)
		}
		if len(cfg.AgentArgsOverride) > 0 {
			detail += "native argument overrides take precedence"
		}
		result.Profiles = append(result.Profiles, ipc.ModelProfileOption{Name: name, Detail: detail})
	}
	return result, nil
}

func (m *RunManager) ForgetModels(p ipc.ForgetModelsParams) error {
	if len(p.Steps) == 0 {
		return fmt.Errorf("name the custom gates to forget")
	}
	for _, step := range p.Steps {
		if !types.StepName(step).IsCustomGate() {
			return fmt.Errorf("invalid custom gate %q", step)
		}
	}
	run, err := m.db.GetRun(p.RunID)
	if err != nil {
		return err
	}
	if run == nil {
		return fmt.Errorf("run not found")
	}
	lock, _ := m.branchLocks.LoadOrStore(run.RepoID+"/"+run.Branch, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	return m.db.DeleteRepoModelChoices(run.RepoID, p.Steps)
}

func (m *RunManager) ConfigureModels(ctx context.Context, p ipc.ConfigureModelsParams) error {
	if m.shuttingDown.Load() {
		return fmt.Errorf("daemon is shutting down")
	}
	run, err := m.db.GetRun(p.RunID)
	if err != nil {
		return err
	}
	if run == nil {
		return fmt.Errorf("run not found")
	}
	lock, _ := m.branchLocks.LoadOrStore(run.RepoID+"/"+run.Branch, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	run, err = m.db.GetRun(p.RunID)
	if err != nil {
		return err
	}
	if run.Status != types.RunPending {
		return fmt.Errorf("run is no longer waiting for model configuration")
	}
	old, err := m.db.GetRunModelPlan(run.ID)
	if err != nil {
		return err
	}
	pin, err := config.ParseStepProfilePlan(old)
	if err != nil {
		return err
	}
	if pin == nil || len(pin.Missing()) == 0 {
		return fmt.Errorf("run has no unresolved model choices")
	}
	missing := pin.Missing()
	if len(p.Choices) != len(missing) {
		return fmt.Errorf("choose a profile for every missing gate")
	}
	global, err := config.LoadGlobal(m.paths.ConfigFile())
	if err != nil {
		return err
	}
	cfg := config.Merge(global, &config.RepoConfig{})
	if p.Token == "" || p.Token != modelPlanToken(old, cfg) {
		return fmt.Errorf("model setup or local profiles changed; refresh before saving")
	}
	if cfg.AgentArgsDigest() != pin.RawArgsDigest {
		return fmt.Errorf("native agent arguments changed; restore them or start a new run")
	}
	for _, step := range missing {
		name, ok := p.Choices[string(step)]
		if !ok {
			return fmt.Errorf("missing choice for %s", step)
		}
		selected, err := cfg.NamedProfile(name)
		if err != nil {
			return err
		}
		pin.Steps[step] = selected
	}
	data, err := json.Marshal(pin)
	if err != nil {
		return err
	}
	// Validate the worktree and all agents before committing choices. Reuse
	// the same pinned routing constructor as ordinary runs.
	pending, err := m.preparePendingModelRun(ctx, run)
	if err != nil {
		return err
	}
	effective, err := m.loadRecoveredConfig(ctx, run, pending.repo, pending.workDir)
	if err != nil {
		return err
	}
	effective.StepProfiles = pin
	forge, err := forgecontext.Resolve(ctx, effective.ForgeProfiles, pending.repo.UpstreamURL, pending.repo.ForkURL)
	if err != nil {
		return err
	}
	ag, err := newPipelineAgent(ctx, effective, m.paths.EvidenceRoot(effective.Test.Evidence.LocalRoot), exec.LookPath, forgeEnvironment(forge))
	if err != nil {
		return err
	}
	if err := m.db.SaveModelChoices(run.ID, run.RepoID, old, string(data), p.Choices); err != nil {
		ag.Close()
		return err
	}
	pending.configuring = false
	pending.cfg = effective
	pending.forge = forge
	pending.agent = ag
	pending.steps = steps.WithCustomGates(m.steps(), effective.Gates)
	m.resumeRecoveredRun(*pending)
	m.broadcast(ipc.Event{Type: ipc.EventRunUpdated, RunID: run.ID})
	return nil
}

func (m *RunManager) cancelPendingModels(run *db.Run, reason string) (bool, error) {
	if run == nil || run.Status != types.RunPending {
		return false, nil
	}
	data, err := m.db.GetRunModelPlan(run.ID)
	if err != nil {
		return false, err
	}
	if data == "" {
		return false, nil
	}
	dir := worktrees.RecordedDir(m.paths, run.WorktreePath(), run.RepoID, run.ID)
	head, verified := preserveRunHead(m.db, dir, run)
	if verified {
		err = m.db.UpdateRunErrorStatusWithVerifiedHead(run.ID, reason, types.RunCancelled, head)
	} else {
		err = m.db.UpdateRunErrorStatus(run.ID, reason, types.RunCancelled)
	}
	if err != nil {
		return false, err
	}
	m.removeRunWorktree(run.RepoID, run.ID, m.paths.RepoDir(run.RepoID), dir, "model_setup_cancelled")
	m.broadcast(ipc.Event{Type: ipc.EventRunUpdated, RunID: run.ID})
	m.closeSubscribers(run.ID)
	return true, nil
}
