package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/BertramPetersen/gatehouse/internal/ipc"
	"github.com/BertramPetersen/gatehouse/internal/types"
	"github.com/spf13/cobra"
)

func TestModelSetupStopsDriveEvenWithYes(t *testing.T) {
	source := &scriptedRunStateSource{
		subscriptions: []scriptedSubscription{{events: make(chan ipc.Event)}},
		runs:          []*ipc.RunInfo{{ID: "run-1", Status: types.RunPending, ModelSetup: []types.StepName{"gate.test.custom"}}},
	}
	reconciler := newRunReconciler(source, "run-1")
	defer reconciler.Close()
	run, ready, err := driveRunWithReconciler(context.Background(), io.Discard, nil, reconciler, "run-1", true)
	if err != nil || ready || run == nil || len(run.ModelSetup) != 1 {
		t.Fatalf("drive: %+v %v %v", run, ready, err)
	}
}

func TestModelSetupDriveResultAsksOperator(t *testing.T) {
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	run := &ipc.RunInfo{ID: "run-1", Status: types.RunPending, ModelSetup: []types.StepName{"gate.test.security"}}
	if err := renderDriveResult(cmd, run, false); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"model-configuration-required", "gate.test.security", "gatehouse axi models --run run-1", "Ask the user"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q in %s", want, out.String())
		}
	}
}

func TestParseModelChoicesRequiresExplicitUniqueAssignments(t *testing.T) {
	for _, invalid := range [][]string{{"gate.test.security"}, {"gate.test.security="}, {"review=fast"}, {"gate.test.security=fast", "gate.test.security=thorough"}} {
		if _, err := parseModelChoices(invalid); err == nil {
			t.Fatalf("accepted %v", invalid)
		}
	}
	got, err := parseModelChoices([]string{"gate.test.security=thorough"})
	if err != nil || got["gate.test.security"] != "thorough" {
		t.Fatalf("got %v, %v", got, err)
	}
}
