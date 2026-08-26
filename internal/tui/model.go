package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"vmcli/internal/api"
	"vmcli/internal/cluster"
	"vmcli/internal/config"
	"vmcli/internal/models"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type refreshMsg struct {
	nodes       []models.Node
	refreshedAt time.Time
}

type screenMode int

const (
	screenList screenMode = iota
	screenDetail
	screenSearch
	screenCommand
	screenHelp
)

type viewPreset string

const (
	viewCluster viewPreset = "cluster"
	viewNodes   viewPreset = "nodes"
	viewInsert  viewPreset = "insert"
	viewSelect  viewPreset = "select"
	viewStorage viewPreset = "storage"
	viewHealth  viewPreset = "health"
)

var commandModes = []string{"cluster", "nodes", "insert", "select", "storage", "health"}

type Model struct {
	client    api.VictoriaMetricsClient
	cluster   config.Cluster
	clusterID string
	interval  time.Duration

	nodes        []models.Node
	selected     int
	width        int
	height       int
	mode         screenMode
	view         viewPreset
	search       string
	searchBackup string
	command      string
	lastRefresh  time.Time
	notice       string
}

func New(client api.VictoriaMetricsClient, selected config.Cluster, clusterID string, interval time.Duration) Model {
	return Model{
		client:    client,
		cluster:   selected,
		clusterID: clusterID,
		interval:  interval,
		width:     120,
		height:    40,
		view:      viewCluster,
	}
}

func (m Model) Init() tea.Cmd { return m.refresh() }

func (m Model) refresh() tea.Cmd {
	return func() tea.Msg {
		nodes := sortNodes(cluster.Check(context.Background(), m.client, cluster.Nodes(m.cluster)))
		return refreshMsg{nodes: nodes, refreshedAt: time.Now()}
	}
}

func (m Model) tick() tea.Cmd {
	if m.interval <= 0 {
		return nil
	}
	return tea.Tick(m.interval, func(time.Time) tea.Msg {
		nodes := sortNodes(cluster.Check(context.Background(), m.client, cluster.Nodes(m.cluster)))
		return refreshMsg{nodes: nodes, refreshedAt: time.Now()}
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.normalizeSelection()
		return m, nil
	case refreshMsg:
		previous := m.selectedNodeKey()
		m.nodes = msg.nodes
		m.lastRefresh = msg.refreshedAt
		m.restoreSelection(previous)
		return m, m.tick()
	case tea.KeyMsg:
		return m.handleKey(msg)
	default:
		return m, nil
	}
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case screenHelp:
		if msg.String() == "esc" || msg.String() == "?" {
			m.mode = screenList
		}
		return m, nil
	case screenDetail:
		return m.handleDetailKey(msg)
	case screenSearch:
		return m.handleSearchKey(msg)
	case screenCommand:
		return m.handleCommandKey(msg)
	default:
		return m.handleListKey(msg)
	}
}

func (m Model) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		m.moveSelection(-1)
	case "down", "j":
		m.moveSelection(1)
	case "g":
		m.selected = 0
	case "G":
		m.selected = len(m.visibleNodes()) - 1
	case "enter":
		if len(m.visibleNodes()) > 0 {
			m.mode = screenDetail
		}
	case "r":
		return m, m.refresh()
	case "/":
		m.mode = screenSearch
		m.searchBackup = m.search
	case ":":
		m.mode = screenCommand
		m.command = ""
	case "?":
		m.mode = screenHelp
	case "n":
		m.moveSelection(1)
	case "N":
		m.moveSelection(-1)
	}
	m.normalizeSelection()
	return m, nil
}

func (m Model) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.mode = screenList
	case "r":
		return m, m.refresh()
	case "up", "k":
		m.moveSelection(-1)
	case "down", "j":
		m.moveSelection(1)
	}
	m.normalizeSelection()
	return m, nil
}

func (m Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.search = m.searchBackup
		m.mode = screenList
		m.normalizeSelection()
		return m, nil
	case tea.KeyEnter:
		m.mode = screenList
		m.normalizeSelection()
		return m, nil
	case tea.KeyBackspace:
		if len(m.search) > 0 {
			m.search = m.search[:len(m.search)-1]
		}
	case tea.KeyRunes:
		m.search += string(msg.Runes)
	}
	m.selected = 0
	m.normalizeSelection()
	return m, nil
}

func (m Model) handleCommandKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.mode = screenList
		m.command = ""
		return m, nil
	case tea.KeyEnter:
		cmd := strings.TrimSpace(m.command)
		m.command = ""
		m.mode = screenList
		if err := m.executeCommand(cmd); err != nil {
			m.notice = err.Error()
		} else {
			m.notice = ""
		}
		m.normalizeSelection()
		return m, nil
	case tea.KeyBackspace:
		if len(m.command) > 0 {
			m.command = m.command[:len(m.command)-1]
		}
	case tea.KeyTab:
		m.command = completeCommand(m.command)
	case tea.KeyRunes:
		m.command += string(msg.Runes)
	}
	return m, nil
}

func (m *Model) executeCommand(cmd string) error {
	switch strings.ToLower(cmd) {
	case "cluster", "nodes":
		if cmd == "cluster" {
			m.view = viewCluster
		} else {
			m.view = viewNodes
		}
		return nil
	case "insert":
		m.view = viewInsert
		return nil
	case "select":
		m.view = viewSelect
		return nil
	case "storage":
		m.view = viewStorage
		return nil
	case "health":
		m.view = viewHealth
		return nil
	case "":
		return nil
	default:
		return fmt.Errorf("unknown command: %s", cmd)
	}
}

func completeCommand(input string) string {
	trimmed := strings.TrimSpace(input)
	lower := strings.ToLower(trimmed)
	for _, candidate := range commandModes {
		if strings.HasPrefix(candidate, lower) {
			return candidate
		}
	}
	return trimmed
}

func (m *Model) moveSelection(delta int) {
	nodes := m.visibleNodes()
	if len(nodes) == 0 {
		m.selected = 0
		return
	}
	m.selected += delta
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= len(nodes) {
		m.selected = len(nodes) - 1
	}
}

func (m *Model) normalizeSelection() {
	nodes := m.visibleNodes()
	if len(nodes) == 0 {
		m.selected = 0
		return
	}
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= len(nodes) {
		m.selected = len(nodes) - 1
	}
}

func (m Model) selectedNodeKey() string {
	nodes := m.visibleNodes()
	if len(nodes) == 0 || m.selected < 0 || m.selected >= len(nodes) {
		return ""
	}
	return nodeKey(nodes[m.selected])
}

func (m *Model) restoreSelection(previous string) {
	nodes := m.visibleNodes()
	if len(nodes) == 0 {
		m.selected = 0
		return
	}
	if previous != "" {
		for index, node := range nodes {
			if nodeKey(node) == previous {
				m.selected = index
				return
			}
		}
	}
	if m.selected >= len(nodes) {
		m.selected = len(nodes) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
}

func (m Model) visibleNodes() []models.Node {
	nodes := append([]models.Node(nil), m.nodes...)
	filtered := make([]models.Node, 0, len(nodes))
	for _, node := range nodes {
		if !matchesView(node, m.view) {
			continue
		}
		if m.search != "" && !matchesSearch(node, m.search) {
			continue
		}
		filtered = append(filtered, node)
	}
	return filtered
}

func matchesView(node models.Node, view viewPreset) bool {
	switch view {
	case viewInsert:
		return node.Component == models.Vminsert
	case viewSelect:
		return node.Component == models.Vmselect
	case viewStorage:
		return node.Component == models.Vmstorage
	default:
		return true
	}
}

func matchesSearch(node models.Node, term string) bool {
	needle := strings.ToLower(strings.TrimSpace(term))
	if needle == "" {
		return true
	}
	values := []string{
		string(node.Component),
		node.URL,
		node.Status,
		node.Version,
		node.Uptime,
		node.Error,
	}
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), needle) {
			return true
		}
	}
	for metric := range node.Metrics {
		if strings.Contains(strings.ToLower(metric), needle) {
			return true
		}
	}
	return false
}

func sortNodes(nodes []models.Node) []models.Node {
	result := append([]models.Node(nil), nodes...)
	sort.SliceStable(result, func(i, j int) bool {
		left := result[i]
		right := result[j]
		if healthView(left.Status) != healthView(right.Status) {
			return healthView(left.Status) < healthView(right.Status)
		}
		if left.Component != right.Component {
			return componentOrder(left.Component) < componentOrder(right.Component)
		}
		return left.URL < right.URL
	})
	return result
}

func healthView(status string) int {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "DOWN":
		return 0
	case "WARNING":
		return 1
	case "UNKNOWN", "":
		return 2
	default:
		return 3
	}
}

func componentOrder(component models.Component) int {
	switch component {
	case models.Vminsert:
		return 0
	case models.Vmselect:
		return 1
	case models.Vmstorage:
		return 2
	default:
		return 3
	}
}

func nodeKey(node models.Node) string {
	return string(node.Component) + "|" + node.URL
}

func shortName(url string) string {
	host := strings.TrimSpace(url)
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimPrefix(host, "https://")
	if index := strings.Index(host, "/"); index >= 0 {
		host = host[:index]
	}
	if index := strings.Index(host, ":"); index >= 0 {
		host = host[:index]
	}
	if index := strings.Index(host, "."); index >= 0 {
		return host[:index]
	}
	return host
}

func componentLabel(component models.Component) string {
	switch component {
	case models.Vminsert:
		return "INSERT"
	case models.Vmselect:
		return "SELECT"
	case models.Vmstorage:
		return "STORAGE"
	default:
		return strings.ToUpper(string(component))
	}
}

func statusText(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "UP":
		return "● UP"
	case "DOWN":
		return "✖ DOWN"
	case "WARNING":
		return "! WARNING"
	case "UNKNOWN", "":
		return "? UNKNOWN"
	default:
		return "? " + strings.ToUpper(strings.TrimSpace(status))
	}
}

func statusStyle(status string) lipgloss.Style {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "UP":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#4ADE80"))
	case "DOWN":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#F87171"))
	case "WARNING":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#FBBF24"))
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#A3A3A3"))
	}
}

func (m Model) headerCounts(nodes []models.Node) (total, up, down, warn, unknown int) {
	for _, node := range nodes {
		switch strings.ToUpper(strings.TrimSpace(node.Status)) {
		case "UP":
			up++
		case "DOWN":
			down++
		case "WARNING":
			warn++
		default:
			unknown++
		}
	}
	return len(nodes), up, down, warn, unknown
}

func (m Model) headerLine(nodes []models.Node) string {
	total, up, down, warn, unknown := m.headerCounts(nodes)
	parts := []string{"vmcli", "VictoriaMetrics Cluster"}
	if strings.TrimSpace(m.clusterID) != "" {
		parts = append(parts, m.clusterID)
	}
	parts = append(parts, fmt.Sprintf("%d nodes", total))
	if up > 0 || total == 0 {
		parts = append(parts, fmt.Sprintf("● %d UP", up))
	}
	if down > 0 {
		parts = append(parts, fmt.Sprintf("✖ %d DOWN", down))
	}
	if warn > 0 {
		parts = append(parts, fmt.Sprintf("! %d WARNING", warn))
	}
	if unknown > 0 {
		parts = append(parts, fmt.Sprintf("? %d UNKNOWN", unknown))
	}
	if m.interval > 0 {
		parts = append(parts, fmt.Sprintf("refresh %s", m.interval))
	}
	if m.search != "" {
		parts = append(parts, fmt.Sprintf("search %q", m.search))
	}
	if m.notice != "" {
		parts = append(parts, m.notice)
	}
	return strings.Join(parts, " ─ ")
}

func (m Model) summaryLine(nodes []models.Node) string {
	total, up, down, warn, unknown := m.headerCounts(nodes)
	parts := []string{fmt.Sprintf("%d nodes", total)}
	if up > 0 || total == 0 {
		parts = append(parts, fmt.Sprintf("%d healthy", up))
	}
	if down > 0 {
		parts = append(parts, fmt.Sprintf("%d down", down))
	}
	if warn > 0 {
		parts = append(parts, fmt.Sprintf("%d warning", warn))
	}
	if unknown > 0 {
		parts = append(parts, fmt.Sprintf("%d unknown", unknown))
	}
	if m.interval > 0 {
		parts = append(parts, fmt.Sprintf("refresh %s", m.interval))
	}
	return strings.Join(parts, " │ ")
}

func (m Model) shortcutsLine() string {
	switch m.mode {
	case screenDetail:
		return "Esc Back  r Refresh  ? Help  q Quit"
	case screenSearch:
		return "/ Search  Enter Apply  Esc Cancel"
	case screenCommand:
		return ": Commands  Tab Complete  Enter Apply  Esc Cancel"
	default:
		return "↑↓/j k Navigate  g/G First/Last  Enter Details  / Search  : Commands  r Refresh  ? Help  q Quit"
	}
}

func (m Model) promptLine() string {
	switch m.mode {
	case screenSearch:
		return fmt.Sprintf("/%s█", m.search)
	case screenCommand:
		return fmt.Sprintf(":%s█", m.command)
	default:
		return ""
	}
}

func (m Model) View() string {
	if m.width <= 0 {
		m.width = 120
	}
	if m.height <= 0 {
		m.height = 40
	}
	nodes := m.visibleNodes()
	if m.mode == screenHelp {
		return m.renderHelp()
	}
	if m.mode == screenDetail {
		return m.renderDetail(nodes)
	}
	return m.renderList(nodes)
}

func (m Model) renderList(nodes []models.Node) string {
	header := m.headerLine(nodes)
	summary := m.summaryLine(nodes)
	shortcuts := m.shortcutsLine()
	prompt := m.promptLine()

	footerLines := []string{summary, shortcuts}
	if prompt != "" {
		footerLines = append([]string{prompt}, footerLines...)
	}

	contentHeight := m.height - 1 - len(footerLines) - 2
	if contentHeight < 4 {
		contentHeight = 4
	}

	content := []string{
		"  " + strings.ToUpper(string(m.viewTitle())),
		"  TYPE      NAME          HOST                         STATUS",
		"  " + strings.Repeat("─", clampWidth(m.width-4, 12)),
	}
	visibleRows := m.renderRows(nodes, contentHeight-len(content))
	content = append(content, visibleRows...)
	for len(content) < contentHeight {
		content = append(content, "")
	}

	screen := make([]string, 0, m.height)
	screen = append(screen, truncateWidth(header, m.width))
	screen = append(screen, renderFrameLines(content, m.width)...)
	for _, line := range footerLines {
		screen = append(screen, truncateWidth(line, m.width))
	}
	return strings.Join(screen, "\n")
}

func (m Model) renderRows(nodes []models.Node, maxRows int) []string {
	if len(nodes) == 0 {
		return []string{"  No nodes match the current filters"}
	}
	if maxRows < 1 {
		maxRows = 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= len(nodes) {
		m.selected = len(nodes) - 1
	}
	top := 0
	if len(nodes) > maxRows {
		top = m.selected - maxRows + 1
		if top < 0 {
			top = 0
		}
		if top > len(nodes)-maxRows {
			top = len(nodes) - maxRows
		}
	}
	bottom := top + maxRows
	if bottom > len(nodes) {
		bottom = len(nodes)
	}
	rows := make([]string, 0, bottom-top)
	for index := top; index < bottom; index++ {
		node := nodes[index]
		rows = append(rows, m.formatRow(node, index == m.selected))
	}
	return rows
}

func (m Model) formatRow(node models.Node, selected bool) string {
	innerWidth := clampWidth(m.width-4, 20)
	includeVersion := innerWidth >= 110
	typeWidth := 8
	nameWidth := 12
	statusWidth := 10
	versionWidth := 8
	used := typeWidth + nameWidth + statusWidth + 6
	if includeVersion {
		used += versionWidth + 2
	}
	hostWidth := innerWidth - used
	if hostWidth < 12 {
		hostWidth = 12
	}
	if includeVersion && innerWidth < used+12 {
		includeVersion = false
		used = typeWidth + nameWidth + statusWidth + 4
		hostWidth = innerWidth - used
		if hostWidth < 12 {
			hostWidth = 12
		}
	}
	parts := []string{
		padRight(componentLabel(node.Component), typeWidth),
		padRight(shortName(node.URL), nameWidth),
		padRight(truncateWidth(node.URL, hostWidth), hostWidth),
		padRight(statusText(node.Status), statusWidth),
	}
	if includeVersion {
		version := node.Version
		if version == "" {
			version = "—"
		}
		parts = append(parts, padRight(version, versionWidth))
	}
	marker := "  "
	if selected {
		marker = lipgloss.NewStyle().Foreground(lipgloss.Color("#60A5FA")).Bold(true).Render("> ")
	}
	return marker + strings.Join(parts, "  ")
}

func (m Model) renderDetail(nodes []models.Node) string {
	header := m.headerLine(nodes)
	summary := m.summaryLine(nodes)
	shortcuts := m.shortcutsLine()

	footerLines := []string{summary, shortcuts}
	contentHeight := m.height - 1 - len(footerLines) - 2
	if contentHeight < 8 {
		contentHeight = 8
	}

	lines := []string{"  NODE DETAILS"}
	if node := m.selectedNode(nodes); node != nil {
		lines = append(lines,
			"  STATUS      "+statusText(node.Status),
			"  TYPE        "+componentLabel(node.Component),
			"  NAME        "+shortName(node.URL),
			"  HOST        "+node.URL,
		)
		if node.Version != "" {
			lines = append(lines, "  VERSION     "+node.Version)
		}
		lines = append(lines, "  ")
		lines = append(lines, "  METRICS")
		keys := metricKeys(node.Metrics)
		if len(keys) == 0 {
			lines = append(lines, "  (no metrics available)")
		} else {
			available := contentHeight - len(lines) - 1
			if available < 1 {
				available = 1
			}
			for idx, key := range keys {
				if idx >= available {
					lines = append(lines, "  …")
					break
				}
				lines = append(lines, fmt.Sprintf("  %-24s %s", key+":", formatMetricValue(node.Metrics[key])))
			}
		}
		if node.Error != "" {
			lines = append(lines, "  ")
			lines = append(lines, "  ERROR       "+node.Error)
		}
	}

	for len(lines) < contentHeight {
		lines = append(lines, "")
	}

	screen := make([]string, 0, m.height)
	screen = append(screen, truncateWidth(header, m.width))
	screen = append(screen, renderFrameLines(lines, m.width)...)
	for _, line := range footerLines {
		screen = append(screen, truncateWidth(line, m.width))
	}
	return strings.Join(screen, "\n")
}

func (m Model) renderHelp() string {
	lines := []string{
		"  Keyboard Shortcuts",
		"  ↑ ↓ / j k    Navigate",
		"  g / G        First / Last",
		"  Enter        Details",
		"  Esc          Back",
		"  /            Search",
		"  r            Refresh",
		"  :            Commands",
		"  ?            Help",
		"  q            Quit",
	}
	boxWidth := 42
	boxHeight := len(lines) + 2
	left := maxInt(0, (m.width-boxWidth)/2)
	top := maxInt(0, (m.height-boxHeight)/2)
	screen := make([]string, 0, m.height)
	for i := 0; i < top; i++ {
		screen = append(screen, "")
	}
	box := renderSimpleBox(lines, boxWidth)
	for _, line := range strings.Split(box, "\n") {
		screen = append(screen, strings.Repeat(" ", left)+line)
	}
	for len(screen) < m.height {
		screen = append(screen, "")
	}
	return strings.Join(screen, "\n")
}

func (m Model) selectedNode(nodes []models.Node) *models.Node {
	if len(nodes) == 0 || m.selected < 0 || m.selected >= len(nodes) {
		return nil
	}
	return &nodes[m.selected]
}

func viewTitle(view viewPreset) string {
	switch view {
	case viewCluster:
		return "CLUSTER"
	case viewNodes:
		return "NODES"
	case viewInsert:
		return "INSERT"
	case viewSelect:
		return "SELECT"
	case viewStorage:
		return "STORAGE"
	case viewHealth:
		return "HEALTH"
	default:
		return "NODES"
	}
}

func (m Model) viewTitle() string { return viewTitle(m.view) }

func metricKeys(metrics map[string]float64) []string {
	if len(metrics) == 0 {
		return nil
	}
	keys := make([]string, 0, len(metrics))
	for key := range metrics {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func formatMetricValue(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", value), "0"), ".")
}

func renderSimpleBox(lines []string, width int) string {
	if width < 20 {
		width = 20
	}
	var b strings.Builder
	b.WriteString("┌")
	b.WriteString(strings.Repeat("─", width-2))
	b.WriteString("┐\n")
	for _, line := range lines {
		content := truncateWidth(line, width-4)
		b.WriteString("│ ")
		b.WriteString(content)
		b.WriteString(strings.Repeat(" ", maxInt(0, width-4-lipgloss.Width(content))))
		b.WriteString(" │\n")
	}
	b.WriteString("└")
	b.WriteString(strings.Repeat("─", width-2))
	b.WriteString("┘")
	return b.String()
}

func renderFrameLines(lines []string, width int) []string {
	if width < 20 {
		width = 20
	}
	result := make([]string, 0, len(lines)+2)
	result = append(result, "├"+strings.Repeat("─", width-2)+"┤")
	for _, line := range lines {
		contentWidth := width - 4
		content := truncateWidth(line, contentWidth)
		padding := contentWidth - lipgloss.Width(content)
		if padding < 0 {
			padding = 0
		}
		result = append(result, "│ "+content+strings.Repeat(" ", padding)+" │")
	}
	result = append(result, "└"+strings.Repeat("─", width-2)+"┘")
	return result
}

func truncateWidth(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(value) <= width {
		return value
	}
	runes := []rune(value)
	if width <= 1 {
		return string(runes[:1])
	}
	limit := 0
	current := 0
	for range runes {
		current++
		if current > width-1 {
			break
		}
		limit++
	}
	if limit <= 0 {
		return "…"
	}
	return string(runes[:limit]) + "…"
}

func padRight(value string, width int) string {
	value = truncateWidth(value, width)
	gap := width - lipgloss.Width(value)
	if gap <= 0 {
		return value
	}
	return value + strings.Repeat(" ", gap)
}

func clampWidth(value, minimum int) int {
	if value < minimum {
		return minimum
	}
	return value
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
