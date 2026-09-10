package tui

import (
	"fmt"
	"strings"

	"github.com/BertramPetersen/gatehouse/internal/ipc"
	"github.com/BertramPetersen/gatehouse/internal/types"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type modelSetupMsg struct {
	runID string
	setup *ipc.ModelSetupResult
	err   error
	saved bool
}

func (m Model) needsModels() bool {
	return m.run != nil && m.run.Status == types.RunPending && len(m.run.ModelSetup) > 0
}

func (m Model) fetchModelsCmd() tea.Cmd {
	if !m.needsModels() || m.client == nil {
		return nil
	}
	return func() tea.Msg {
		var result ipc.ModelSetupResult
		err := m.client.Call(ipc.MethodModelSetup, &ipc.GetRunParams{RunID: m.run.ID}, &result)
		return modelSetupMsg{runID: m.run.ID, setup: &result, err: err}
	}
}

func (m Model) handleModelKey(key string) (tea.Model, tea.Cmd) {
	if m.modelSaving || m.modelLoading {
		return m, nil
	}
	if key == "r" {
		m.modelLoading = true
		return m, m.fetchModelsCmd()
	}
	if m.modelSetup == nil || len(m.modelSetup.Missing) == 0 || len(m.modelSetup.Profiles) == 0 {
		return m, nil
	}
	gates, profiles := m.modelSetup.Missing, m.modelSetup.Profiles
	switch key {
	case "up", "k":
		m.modelGate = (m.modelGate + len(gates) - 1) % len(gates)
	case "down", "j", "tab":
		m.modelGate = (m.modelGate + 1) % len(gates)
	case "left", "right", "h", "l", " ":
		step := string(gates[m.modelGate])
		index := -1
		for i, profile := range profiles {
			if profile.Name == m.modelChoices[step] {
				index = i
			}
		}
		if key == "left" || key == "h" {
			index--
			if index < 0 {
				index = len(profiles) - 1
			}
		} else {
			index = (index + 1) % len(profiles)
		}
		m.modelChoices[step] = profiles[index].Name
		m.err = nil
	case "enter":
		if len(m.modelChoices) != len(gates) {
			m.err = fmt.Errorf("choose a profile for all %d gates before saving", len(gates))
			return m, nil
		}
		if m.client == nil {
			return m, nil
		}
		choices := make(map[string]string, len(m.modelChoices))
		for step, name := range m.modelChoices {
			choices[step] = name
		}
		params := ipc.ConfigureModelsParams{RunID: m.run.ID, Token: m.modelSetup.Token, Choices: choices}
		m.modelSaving = true
		return m, func() tea.Msg {
			var result ipc.RespondResult
			err := m.client.Call(ipc.MethodConfigureModels, &params, &result)
			return modelSetupMsg{runID: params.RunID, saved: err == nil, err: err}
		}
	}
	return m, nil
}

func (m Model) modelSetupView() string {
	var b strings.Builder
	b.WriteString("Choose models for this repository\n\nLocal choices only — no pipeline steps have started.\n")
	if m.modelSetup == nil || len(m.modelSetup.Missing) == 0 {
		b.WriteString("\nLoading local profiles…\n")
	} else {
		gates := m.modelSetup.Missing
		step := string(gates[m.modelGate])
		fmt.Fprintf(&b, "\nGate %d/%d: %s\nChosen: %d/%d\n\n", m.modelGate+1, len(gates), step, len(m.modelChoices), len(gates))
		name := m.modelChoices[step]
		if name == "" {
			b.WriteString("←/→ Choose a profile (none selected)\n")
		} else {
			for _, profile := range m.modelSetup.Profiles {
				if profile.Name == name {
					fmt.Fprintf(&b, "← %s →\n%s\n", profile.Name, profile.Detail)
				}
			}
		}
	}
	if m.err != nil {
		fmt.Fprintf(&b, "\n%s\n", m.err)
	}
	if m.modelSaving {
		b.WriteString("\nSaving choices…\n")
	}
	if m.confirmAbort {
		b.WriteString("\nPress x again to abort.\n")
	}
	b.WriteString("\n↑/↓ gate · ←/→ profile · enter save all\nr refresh · x abort · q detach")
	width := m.width - 4
	if width < 20 {
		width = 20
	}
	return lipgloss.NewStyle().Width(width).Padding(1, 2).Render(b.String())
}
