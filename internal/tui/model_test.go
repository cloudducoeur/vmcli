package tui

import (
	"strings"
	"testing"
	"time"

	"vmcli/internal/config"
	"vmcli/internal/models"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTUIListViewRendersSummaryAndSelection(t *testing.T) {
	model := New(nil, config.Cluster{}, "production", 5*time.Second)
	model.nodes = []models.Node{
		{Component: models.Vminsert, URL: "vminsert1.internal.rdcnet.org:8480", Status: "UP"},
		{Component: models.Vmselect, URL: "vmselect1.internal.rdcnet.org:8481", Status: "DOWN"},
		{Component: models.Vmstorage, URL: "vmstorage1.internal.rdcnet.org:8482", Status: "UP"},
	}
	model.width = 120
	model.height = 30
	model.selected = 1

	view := model.View()
	for _, want := range []string{"VictoriaMetrics Cluster", "production", "3 nodes", "● 2 UP", "✖ 1 DOWN", "TYPE", "STATUS", "vmselect1"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q in view:\n%s", want, view)
		}
	}
}

func TestTUISearchAndCommandModes(t *testing.T) {
	model := New(nil, config.Cluster{}, "production", 5*time.Second)
	model.nodes = []models.Node{
		{Component: models.Vminsert, URL: "vminsert1.internal.rdcnet.org:8480", Status: "UP"},
		{Component: models.Vmstorage, URL: "vmstorage1.internal.rdcnet.org:8482", Status: "UP"},
	}
	model.width = 120
	model.height = 30

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = next.(Model)
	if model.mode != screenSearch {
		t.Fatalf("expected search mode, got %v", model.mode)
	}
	for _, r := range "storage" {
		next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		model = next.(Model)
	}
	if got := len(model.visibleNodes()); got != 1 {
		t.Fatalf("expected 1 search result, got %d", got)
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if model.mode != screenList {
		t.Fatalf("expected list mode after search enter, got %v", model.mode)
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}})
	model = next.(Model)
	for _, r := range "insert" {
		next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		model = next.(Model)
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if model.view != viewInsert {
		t.Fatalf("expected insert view, got %v", model.view)
	}
}

func TestTUIDetailAndHelpScreens(t *testing.T) {
	model := New(nil, config.Cluster{}, "production", 5*time.Second)
	model.nodes = []models.Node{
		{Component: models.Vmstorage, URL: "vmstorage1.internal.rdcnet.org:8482", Status: "UP", Metrics: map[string]float64{"process_cpu_seconds_total": 12.5}},
	}
	model.width = 120
	model.height = 30

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if model.mode != screenDetail {
		t.Fatalf("expected detail mode, got %v", model.mode)
	}
	view := model.View()
	for _, want := range []string{"NODE DETAILS", "STATUS", "METRICS", "process_cpu_seconds_total"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q in detail view:\n%s", want, view)
		}
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = next.(Model)
	if model.mode != screenList {
		t.Fatalf("expected list mode after escape, got %v", model.mode)
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	model = next.(Model)
	if model.mode != screenHelp {
		t.Fatalf("expected help mode, got %v", model.mode)
	}
	helpView := model.View()
	if !strings.Contains(helpView, "Keyboard Shortcuts") || !strings.Contains(helpView, "Navigate") {
		t.Fatalf("expected help popup, got:\n%s", helpView)
	}
}
