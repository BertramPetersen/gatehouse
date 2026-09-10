package cli

import (
	"fmt"
	"strings"

	"github.com/BertramPetersen/gatehouse/internal/ipc"
	"github.com/BertramPetersen/gatehouse/internal/types"
	"github.com/spf13/cobra"
	toon "github.com/toon-format/toon-go"
)

const modelSetupGuidance = "Ask the user which local profile to use for every listed gate. Do not infer model choices or use --yes to bypass setup."

func modelStepNames(steps []types.StepName) []string {
	names := make([]string, len(steps))
	for i, step := range steps {
		names[i] = string(step)
	}
	return names
}

func parseModelChoices(values []string) (map[string]string, error) {
	choices := map[string]string{}
	for _, value := range values {
		step, profile, ok := strings.Cut(value, "=")
		if !ok || !types.StepName(step).IsCustomGate() || !types.ValidCustomGateLabel(profile) {
			return nil, fmt.Errorf("expected --set gate.<step>.<name>=<local-profile>, got %q", value)
		}
		if _, exists := choices[step]; exists {
			return nil, fmt.Errorf("duplicate model choice for %s", step)
		}
		choices[step] = profile
	}
	return choices, nil
}

func newAxiModelsCmd() *cobra.Command {
	var runID string
	var assignments []string
	var forget []string
	cmd := &cobra.Command{
		Use: "models", Short: "Inspect and explicitly configure a run's missing local model choices",
		Long: "List missing custom-gate profiles and available local choices. Repeated --set flags save all missing choices atomically and start the same pending run. " + modelSetupGuidance,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if len(forget) > 0 && len(assignments) > 0 {
				return emitError(cmd, 1, "use --forget or --set, not both")
			}
			choices, err := parseModelChoices(assignments)
			if err != nil {
				return emitError(cmd, 1, err.Error())
			}
			env, err := openAxiEnvWithOptions(axiEnvOptions{ensureDaemonConn: true, explicitRunID: runID})
			if err != nil {
				return emitError(cmd, 1, err.Error())
			}
			defer env.close()
			if len(forget) > 0 {
				var result ipc.RespondResult
				if err := env.client.Call(ipc.MethodForgetModels, &ipc.ForgetModelsParams{RunID: runID, Steps: forget}, &result); err != nil {
					return emitError(cmd, 1, err.Error())
				}
				emitDoc(cmd, toon.Field{Key: "outcome", Value: "models-forgotten"}, toon.Field{Key: "help", Value: []string{"Future runs will ask again. Existing runs keep their pinned models."}})
				return nil
			}
			var setup ipc.ModelSetupResult
			if err := env.client.Call(ipc.MethodModelSetup, &ipc.GetRunParams{RunID: runID}, &setup); err != nil {
				return emitError(cmd, 1, err.Error())
			}
			if len(assignments) != 0 {
				var result ipc.RespondResult
				if err := env.client.Call(ipc.MethodConfigureModels, &ipc.ConfigureModelsParams{RunID: runID, Token: setup.Token, Choices: choices}, &result); err != nil {
					return emitError(cmd, 1, err.Error())
				}
				emitDoc(cmd, toon.Field{Key: "outcome", Value: "models-configured"}, toon.Field{Key: "run", Value: runID}, toon.Field{Key: "help", Value: []string{"Observe with gatehouse axi status --run " + runID + "; gatehouse axi run reattaches from the original branch."}})
				return nil
			}
			emitDoc(cmd, toon.Field{Key: "run", Value: runID}, toon.Field{Key: "saved", Value: setup.Saved}, toon.Field{Key: "missing", Value: modelStepNames(setup.Missing)}, toon.Field{Key: "profiles", Value: setup.Profiles}, toon.Field{Key: "help", Value: []string{modelSetupGuidance, "Save with gatehouse axi models --run " + runID + " --set gate.<step>.<name>=<profile> (repeat for every missing gate)."}})
			return nil
		},
	}
	cmd.Flags().StringVar(&runID, "run", "", "Run ID")
	cmd.Flags().StringArrayVar(&assignments, "set", nil, "Explicit custom-gate profile assignment (repeatable)")
	cmd.Flags().StringArrayVar(&forget, "forget", nil, "Forget a saved gate choice for future runs in this repository (repeatable)")
	_ = cmd.MarkFlagRequired("run")
	return cmd
}
