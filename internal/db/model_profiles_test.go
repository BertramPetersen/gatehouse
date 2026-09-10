package db

import "testing"

func TestModelChoicesCommitWithRunPinAndRejectStaleSave(t *testing.T) {
	d := openTestDB(t)
	repo, err := d.InsertRepo("/models", "https://example.com/models", "main")
	if err != nil {
		t.Fatal(err)
	}
	run, err := d.InsertRun(repo.ID, "feature", "head", "base")
	if err != nil {
		t.Fatal(err)
	}
	if err := d.InsertRunModelPlan(run.ID, "pending"); err != nil {
		t.Fatal(err)
	}
	choices := map[string]string{"gate.test.architecture": "economical"}
	if err := d.SaveModelChoices(run.ID, repo.ID, "pending", "ready", choices); err != nil {
		t.Fatal(err)
	}
	if err := d.SaveModelChoices(run.ID, repo.ID, "pending", "stale", map[string]string{"gate.test.architecture": "thorough"}); err == nil {
		t.Fatal("accepted stale write")
	}
	got, err := d.GetRepoModelChoices(repo.ID)
	if err != nil || got["gate.test.architecture"] != "economical" {
		t.Fatalf("choices: %v %v", got, err)
	}
	if got, err := d.GetRunModelPlan(run.ID); err != nil || got != "ready" {
		t.Fatalf("pin: %s %v", got, err)
	}
	if err := d.DeleteRepoModelChoices(repo.ID, []string{"gate.test.architecture"}); err != nil {
		t.Fatal(err)
	}
	got, _ = d.GetRepoModelChoices(repo.ID)
	if len(got) != 0 {
		t.Fatal("choice not forgotten")
	}
	if got, _ := d.GetRunModelPlan(run.ID); got != "ready" {
		t.Fatal("forget rewrote existing run")
	}
	if err := d.SaveModelChoices(run.ID, repo.ID, "ready", "next", choices); err != nil {
		t.Fatal(err)
	}
	if err := d.DeleteRepo(repo.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = d.GetRepoModelChoices(repo.ID)
	if len(got) != 0 {
		t.Fatal("eject retained choices")
	}
	if got, _ := d.GetRunModelPlan(run.ID); got != "" {
		t.Fatal("eject retained pin")
	}
}
