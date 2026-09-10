package daemon

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/BertramPetersen/gatehouse/internal/config"
	"github.com/BertramPetersen/gatehouse/internal/db"
	"github.com/BertramPetersen/gatehouse/internal/ipc"
	"github.com/BertramPetersen/gatehouse/internal/paths"
	"github.com/BertramPetersen/gatehouse/internal/pipeline"
	"github.com/BertramPetersen/gatehouse/internal/types"
	"github.com/BertramPetersen/gatehouse/internal/worktrees"
)

func modelSetupFixture(t *testing.T) (*RunManager, *db.Repo, *db.Run, *mockPassStep) {
	t.Helper()
	p := paths.WithRoot(t.TempDir())
	if err := p.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	mock := writeMockClaude(t, t.TempDir())
	if err := os.WriteFile(p.ConfigFile(), []byte("agent: claude\nagent_path_override:\n  claude: "+mock+"\nagent_profiles:\n  economical:\n    claude: {model: small, effort: low}\n  thorough:\n    claude: {model: large, effort: high}\nagent_step_profiles:\n  review: thorough\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := db.Open(p.DB())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	repo, _ := setupTestGitRepo(t, p, d, "models")
	head := commitDefaultBranchConfig(t, repo.WorkingPath, "gates:\n  - name: arch-fitness\n    after: test\n    instructions: Check imports\n  - name: mutation-budget\n    after: test\n    instructions: Check tests\n")
	step := &mockPassStep{name: types.StepReview}
	m := NewRunManager(d, p, func() []pipeline.Step { return []pipeline.Step{step} })
	t.Cleanup(m.Shutdown)
	id, err := m.startRunWithIntentSource(context.Background(), repo, "feature", head, head, "test", nil, "exercise model choices", "agent")
	if err != nil {
		t.Fatal(err)
	}
	run, err := d.GetRun(id)
	if err != nil {
		t.Fatal(err)
	}
	return m, repo, run, step
}

func TestModelSetupParksRecoversAndStartsSameRun(t *testing.T) {
	m, repo, run, step := modelSetupFixture(t)
	if run.Status != types.RunPending || step.execCnt.Load() != 0 {
		t.Fatal("setup executed a step")
	}
	rows, err := m.db.GetStepsByRun(run.ID)
	if err != nil || len(rows) != 0 {
		t.Fatalf("steps: %v %v", rows, err)
	}
	plans := m.recoverableParkedRuns(context.Background())
	if len(plans) != 1 || !plans[0].configuring {
		t.Fatalf("setup not recoverable: %+v", plans)
	}
	m.resumeRecoveredRuns(plans)
	recoverOnStartup(m.db, m.paths, m, worktrees.New(m.paths, nil))
	storedRun, err := m.db.GetRun(run.ID)
	if err != nil || storedRun.Status != types.RunPending {
		t.Fatalf("startup discarded setup: %+v %v", storedRun, err)
	}
	if _, err := os.Stat(m.paths.WorktreeDir(run.RepoID, run.ID)); err != nil {
		t.Fatalf("startup removed setup worktree: %v", err)
	}
	if step.execCnt.Load() != 0 {
		t.Fatal("recovery bypassed setup")
	}
	setup, err := m.ModelSetup(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(setup.Missing) != 2 || len(setup.Profiles) != 3 {
		t.Fatalf("setup: %+v", setup)
	}
	choices := map[string]string{"gate.test.arch-fitness": "economical", "gate.test.mutation-budget": "economical"}
	partial := ipc.ConfigureModelsParams{RunID: run.ID, Token: setup.Token, Choices: map[string]string{"gate.test.arch-fitness": "economical"}}
	if err := m.ConfigureModels(context.Background(), partial); err == nil {
		t.Fatal("accepted partial batch")
	}
	stored, _ := m.db.GetRepoModelChoices(repo.ID)
	if len(stored) != 0 {
		t.Fatal("partial batch persisted")
	}
	params := ipc.ConfigureModelsParams{RunID: run.ID, Token: setup.Token, Choices: choices}
	if err := m.ConfigureModels(context.Background(), params); err != nil {
		t.Fatal(err)
	}
	waitForRunTerminalState(t, m.db, run.ID)
	if step.execCnt.Load() != 1 {
		t.Fatal("same run did not execute exactly once")
	}
	if err := m.ConfigureModels(context.Background(), params); err == nil {
		t.Fatal("stale save accepted")
	}
	stored, _ = m.db.GetRepoModelChoices(repo.ID)
	if len(stored) != 2 {
		t.Fatal(stored)
	}
	data, _ := m.db.GetRunModelPlan(run.ID)
	pin, err := config.ParseStepProfilePlan(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(pin.Missing()) != 0 || pin.Steps[types.StepReview].Agents["claude"].Model != "large" {
		t.Fatalf("pin: %+v", pin)
	}
	other, _ := m.db.GetRepoModelChoices("other")
	if len(other) != 0 {
		t.Fatal("choices leaked across repos")
	}
	id, err := m.startRunWithIntentSource(context.Background(), repo, "second", run.HeadSHA, run.HeadSHA, "test", nil, "reuse choices", "agent")
	if err != nil {
		t.Fatal(err)
	}
	waitForRunTerminalState(t, m.db, id)
	if step.execCnt.Load() != 2 {
		t.Fatal("saved choices did not avoid prompting")
	}
}

func TestModelSetupCancelPreservesNoRunnableSetup(t *testing.T) {
	m, _, run, step := modelSetupFixture(t)
	if err := m.HandleCancel(run.ID); err != nil {
		t.Fatal(err)
	}
	stored, _ := m.db.GetRun(run.ID)
	if stored.Status != types.RunCancelled || step.execCnt.Load() != 0 {
		t.Fatalf("cancel: %+v", stored)
	}
	if _, err := os.Stat(m.paths.WorktreeDir(run.RepoID, run.ID)); !os.IsNotExist(err) {
		t.Fatalf("worktree retained: %v", err)
	}
	if plans := m.recoverableParkedRuns(context.Background()); len(plans) != 0 {
		t.Fatal("cancelled setup recovered")
	}
}

func TestModelSetupRejectsChangedCatalogAndConcurrentSaves(t *testing.T) {
	m, _, run, step := modelSetupFixture(t)
	setup, err := m.ModelSetup(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(m.paths.ConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.paths.ConfigFile(), []byte(strings.ReplaceAll(string(data), "large", "edited-large")), 0o644); err != nil {
		t.Fatal(err)
	}
	params := ipc.ConfigureModelsParams{RunID: run.ID, Token: setup.Token, Choices: map[string]string{"gate.test.arch-fitness": "economical", "gate.test.mutation-budget": "economical"}}
	if err := m.ConfigureModels(context.Background(), params); err == nil {
		t.Fatal("stale displayed catalog accepted")
	}
	if step.execCnt.Load() != 0 {
		t.Fatal("stale save executed")
	}
	setup, err = m.ModelSetup(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	params.Token = setup.Token
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { results <- m.ConfigureModels(context.Background(), params) }()
	}
	success := 0
	for i := 0; i < 2; i++ {
		if <-results == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("%d saves succeeded", success)
	}
	waitForRunTerminalState(t, m.db, run.ID)
	if step.execCnt.Load() != 1 {
		t.Fatal("concurrent saves launched duplicate runs")
	}
	payload, _ := m.db.GetRunModelPlan(run.ID)
	pin, err := config.ParseStepProfilePlan(payload)
	if err != nil {
		t.Fatal(err)
	}
	if pin.Steps[types.StepReview].Agents["claude"].Model != "large" {
		t.Fatal("catalog edit changed pinned core model")
	}
}
