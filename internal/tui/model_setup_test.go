package tui

import (
	"strings"
	"testing"

	"github.com/BertramPetersen/gatehouse/internal/ipc"
	"github.com/BertramPetersen/gatehouse/internal/types"
)

func TestModelSetupRequiresExplicitChoicesEvenInYolo(t *testing.T) {
	m := NewModel("", nil, &ipc.RunInfo{ID: "run-1", Status: types.RunPending, ModelSetup: []types.StepName{"gate.test.security", "gate.test.arch"}})
	m.yoloMode = true
	next, _ := m.Update(modelSetupMsg{runID: "run-1", setup: &ipc.ModelSetupResult{RunID: "run-1", Missing: m.run.ModelSetup, Profiles: []ipc.ModelProfileOption{{Name: "default"}, {Name: "thorough", Detail: "claude: large"}}}})
	m = next.(Model)
	if cmd := m.maybeAutoApproveCmd(); cmd != nil {
		t.Fatal("auto approval bypassed setup")
	}
	next, cmd := m.handleKey(keyMsg("enter"))
	m = next.(Model)
	if cmd != nil || len(m.modelChoices) != 0 {
		t.Fatal("enter accepted unchosen defaults")
	}
	next, _ = m.handleKey(keyMsg("right"))
	m = next.(Model)
	if len(m.modelChoices) != 1 {
		t.Fatal("must choose only focused gate")
	}
	next, cmd = m.handleKey(keyMsg("enter"))
	m = next.(Model)
	if cmd != nil {
		t.Fatal("saved incomplete choices")
	}
	next, _ = m.handleKey(keyMsg("right"))
	m = next.(Model)
	if !strings.Contains(m.View(), "thorough") {
		t.Fatal(m.View())
	}
	next, _ = m.handleKey(keyMsg("down"))
	m = next.(Model)
	if !strings.Contains(m.View(), "gate.test.arch") {
		t.Fatal(m.View())
	}
}

func TestModelSetupIgnoresOldRunResponses(t *testing.T) {
	m := NewModel("", nil, &ipc.RunInfo{ID: "new", Status: types.RunPending})
	next, _ := m.Update(modelSetupMsg{runID: "old", setup: &ipc.ModelSetupResult{RunID: "old"}})
	if next.(Model).modelSetup != nil {
		t.Fatal("stale setup applied")
	}
}
