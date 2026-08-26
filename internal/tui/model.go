package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"vmcli/internal/api"
	"vmcli/internal/cluster"
	"vmcli/internal/config"
	"vmcli/internal/models"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type refreshMsg struct{ nodes []models.Node }

type Model struct {
	client   api.VictoriaMetricsClient
	cluster  config.Cluster
	interval time.Duration
	nodes    []models.Node
	selected int
	details  bool
	err      error
	width    int
}

func New(client api.VictoriaMetricsClient, selected config.Cluster, interval time.Duration) Model {
	return Model{client: client, cluster: selected, interval: interval}
}
func (m Model) Init() tea.Cmd { return m.refresh() }
func (m Model) refresh() tea.Cmd {
	return func() tea.Msg {
		return refreshMsg{nodes: cluster.Check(context.Background(), m.client, cluster.Nodes(m.cluster))}
	}
}
func (m Model) tick() tea.Cmd {
	return tea.Tick(m.interval, func(time.Time) tea.Msg {
		return refreshMsg{nodes: cluster.Check(context.Background(), m.client, cluster.Nodes(m.cluster))}
	})
}
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.selected < len(m.nodes)-1 {
				m.selected++
			}
		case "enter":
			m.details = true
		case "esc":
			m.details = false
		case "r":
			return m, m.refresh()
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case refreshMsg:
		m.nodes, m.err = msg.nodes, nil
		return m, m.tick()
	}
	return m, nil
}
func (m Model) View() string {
	if m.details && len(m.nodes) > 0 {
		return m.detailView()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7DD3FC")).Render("vmcli  VictoriaMetrics Cluster"))
	b.WriteString("COMPONENT       HOST                         STATUS\n")
	b.WriteString("-----------------------------------------------------------\n")
	for index, node := range m.nodes {
		marker := "  "
		if index == m.selected {
			marker = "> "
		}
		color := lipgloss.Color("#A3A3A3")
		if node.Status == "UP" {
			color = lipgloss.Color("#4ADE80")
		}
		if node.Status == "DOWN" {
			color = lipgloss.Color("#F87171")
		}
		fmt.Fprintf(&b, "%s%-15s %-28s %s\n", marker, node.Component, node.URL, lipgloss.NewStyle().Foreground(color).Render("● "+node.Status))
	}
	if m.err != nil {
		fmt.Fprintf(&b, "\nError: %s\n", m.err)
	}
	b.WriteString("\n[ENTER] Details  [r] Refresh  [q] Quit  [↑/↓ j/k] Navigate")
	return b.String()
}
func (m Model) detailView() string {
	node := m.nodes[m.selected]
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Node Details\n\nComponent: %s\nEndpoint:  %s\nStatus:    %s\n", node.Component, node.URL, node.Status))
	if node.Error != "" {
		fmt.Fprintf(&b, "Error:     %s\n", node.Error)
	}
	b.WriteString("\nMetrics\n")
	for name, value := range node.Metrics {
		b.WriteString(fmt.Sprintf("  %-42s %.3f\n", name, value))
	}
	b.WriteString("\n[ESC] Back  [r] Refresh  [q] Quit")
	return b.String()
}
