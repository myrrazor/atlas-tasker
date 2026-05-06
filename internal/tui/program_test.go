package tui

import (
	"bytes"
	"io"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestProgramLaunchesAndQuits(t *testing.T) {
	root := seededTUIWorkspace(t)
	m, err := newModel(root, contracts.Actor("agent:builder-1"))
	if err != nil {
		t.Fatalf("new model: %v", err)
	}
	defer m.close()
	program := tea.NewProgram(m, tea.WithInput(bytes.NewBufferString("q")), tea.WithOutput(io.Discard), tea.WithoutSignals())
	finalModel, err := program.Run()
	if err != nil {
		t.Fatalf("run tui program: %v", err)
	}
	final := finalModel.(model)
	if final.status == "" {
		t.Fatal("expected final model to carry status after launch")
	}
}
