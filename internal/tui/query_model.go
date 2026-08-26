package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"vmcli/internal/api"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/guptarohit/asciigraph"
)

type QueryFetcher func(context.Context) (api.QueryResult, error)

type queryTickMsg time.Time

type queryRefreshMsg struct {
	result api.QueryResult
	err    error
	at     time.Time
}

type QueryModel struct {
	fetch           QueryFetcher
	query           string
	endpoint        string
	rangeLabel      string
	result          api.QueryResult
	refreshInterval time.Duration
	seriesValues    map[string][]float64
	seriesOrder     []string
	selectedSeries  int
	singleSeries    bool
	seriesCount     int
	totalValue      float64
	lastUpdated     time.Time
	err             error
	width           int
	height          int
	xRangeStart     float64
	xRangeEnd       float64
	hasTimeRange    bool
}

func NewQuery(fetch QueryFetcher, query, endpoint, rangeLabel string, result api.QueryResult, refreshInterval time.Duration) QueryModel {
	model := QueryModel{
		fetch:           fetch,
		query:           query,
		endpoint:        endpoint,
		rangeLabel:      rangeLabel,
		result:          result,
		refreshInterval: refreshInterval,
		seriesValues:    make(map[string][]float64),
		width:           100,
		height:          30,
	}
	model.recordResult(result, time.Now())
	return model
}

func (m QueryModel) Init() tea.Cmd {
	if m.refreshInterval <= 0 {
		return nil
	}
	return m.tick()
}

func (m QueryModel) tick() tea.Cmd {
	return tea.Tick(m.refreshInterval, func(at time.Time) tea.Msg {
		return queryTickMsg(at)
	})
}

func (m QueryModel) refresh() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		result, err := m.fetch(ctx)
		return queryRefreshMsg{result: result, err: err, at: time.Now()}
	}
}

func (m QueryModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "r":
			return m, m.refresh()
		case "s":
			m.singleSeries = !m.singleSeries
		case "left", "h":
			if len(m.seriesOrder) > 0 {
				m.selectedSeries = (m.selectedSeries - 1 + len(m.seriesOrder)) % len(m.seriesOrder)
			}
		case "right", "l":
			if len(m.seriesOrder) > 0 {
				m.selectedSeries = (m.selectedSeries + 1) % len(m.seriesOrder)
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case queryTickMsg:
		return m, m.refresh()
	case queryRefreshMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.err = nil
			m.recordResult(msg.result, msg.at)
		}
		if m.refreshInterval > 0 {
			return m, m.tick()
		}
	}
	return m, nil
}

func (m QueryModel) View() string {
	var b strings.Builder
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7DD3FC")).Render("vmcli  PromQL Query")
	fmt.Fprintf(&b, "%s\n\n", title)
	fmt.Fprintf(&b, "Endpoint: %s\n", m.endpoint)
	fmt.Fprintf(&b, "Query:    %s\n", m.query)
	fmt.Fprintf(&b, "Range:    %s\n", m.rangeLabel)
	fmt.Fprintf(&b, "Status:   %s\n", m.result.Status)
	fmt.Fprintf(&b, "Type:     %s\n", m.result.Data.ResultType)
	if m.refreshInterval > 0 {
		fmt.Fprintf(&b, "Refresh:  every %s\n", m.refreshInterval)
	} else {
		b.WriteString("Refresh:  manual only\n")
	}
	if !m.lastUpdated.IsZero() {
		fmt.Fprintf(&b, "Updated:  %s\n", m.lastUpdated.Format(time.RFC3339))
	}
	if m.seriesCount > 0 {
		fmt.Fprintf(&b, "Series:   %d (sum=%.6g)\n", m.seriesCount, m.totalValue)
	}
	if m.err != nil {
		fmt.Fprintf(&b, "Error:    %s\n", m.err)
	}
	if m.singleSeries && len(m.seriesOrder) > 0 {
		fmt.Fprintf(&b, "Mode:     single series (%s)\n", m.seriesOrder[m.selectedSeries])
	} else {
		b.WriteString("Mode:     all series\n")
	}
	b.WriteString("\nGraph\n")
	b.WriteString("-----------------------------------------------------------\n")
	b.WriteString(m.graphView())
	b.WriteString("\n[r] Refresh  [s] Single/All  [←/→ h/l] Select series  [q] Quit")
	return b.String()
}

func (m QueryModel) graphView() string {
	visible := m.visibleSeries()
	if len(visible) == 0 {
		return "(no numeric series yet)"
	}

	plotWidth := m.width - 8
	if plotWidth < 50 {
		plotWidth = 50
	}
	plotHeight := m.height / 2
	if plotHeight < 8 {
		plotHeight = 8
	}
	if plotHeight > 20 {
		plotHeight = 20
	}

	data := make([][]float64, 0, len(visible))
	legends := make([]string, 0, len(visible))
	colors := make([]asciigraph.AnsiColor, 0, len(visible))
	for _, key := range visible {
		values := m.seriesValues[key]
		if len(values) == 0 {
			continue
		}
		data = append(data, values)
		legends = append(legends, trimLabel(key, 36))
		colors = append(colors, seriesColor(m.seriesIndex(key)))
	}
	if len(data) == 0 {
		return "(no numeric samples yet)"
	}

	options := []asciigraph.Option{
		asciigraph.Width(plotWidth),
		asciigraph.Height(plotHeight),
		asciigraph.Precision(3),
		asciigraph.AxisColor(asciigraph.White),
		asciigraph.LabelColor(asciigraph.White),
		asciigraph.SeriesColors(colors...),
		asciigraph.SeriesLegends(legends...),
		asciigraph.YAxisValueFormatter(func(value float64) string {
			return fmt.Sprintf("%.3g", value)
		}),
	}
	if m.hasTimeRange && m.xRangeEnd > m.xRangeStart {
		span := m.xRangeEnd - m.xRangeStart
		options = append(options,
			asciigraph.XAxisRange(m.xRangeStart, m.xRangeEnd),
			asciigraph.XAxisTickCount(6),
			asciigraph.XAxisValueFormatter(func(value float64) string {
				return formatUnixXAxis(value, span)
			}),
		)
	}

	return asciigraph.PlotMany(data, options...)
}

func (m *QueryModel) recordResult(result api.QueryResult, at time.Time) {
	m.result = result
	m.lastUpdated = at
	seriesValues, xStart, xEnd, hasTimeRange := extractSeriesTimelines(result)
	m.seriesValues = seriesValues
	m.hasTimeRange = hasTimeRange
	m.xRangeStart = xStart
	m.xRangeEnd = xEnd

	order := make([]string, 0, len(seriesValues))
	for key := range seriesValues {
		order = append(order, key)
	}
	sort.Strings(order)
	m.seriesOrder = order

	m.seriesCount = len(seriesValues)
	m.totalValue = 0
	for _, key := range m.seriesOrder {
		values := seriesValues[key]
		if len(values) == 0 {
			continue
		}
		last := values[len(values)-1]
		if !math.IsNaN(last) {
			m.totalValue += last
		}
	}

	if len(m.seriesOrder) == 0 {
		m.selectedSeries = 0
	} else if m.selectedSeries >= len(m.seriesOrder) {
		m.selectedSeries = len(m.seriesOrder) - 1
	}
}

func extractSeriesTimelines(result api.QueryResult) (map[string][]float64, float64, float64, bool) {
	samples := make(map[string][]float64)
	minTs := math.Inf(1)
	maxTs := math.Inf(-1)
	hasTime := false

	for _, raw := range result.Data.Result {
		label, values, start, end, withTime, ok := extractSampleTimeline(raw)
		if !ok || len(values) == 0 {
			continue
		}
		samples[label] = values
		if withTime {
			hasTime = true
			if start < minTs {
				minTs = start
			}
			if end > maxTs {
				maxTs = end
			}
		}
	}

	if !hasTime || math.IsInf(minTs, 1) || math.IsInf(maxTs, -1) || minTs >= maxTs {
		return samples, 0, 0, false
	}
	return samples, minTs, maxTs, true
}

func extractSampleTimeline(raw json.RawMessage) (string, []float64, float64, float64, bool, bool) {
	var vectorSample struct {
		Metric map[string]string   `json:"metric"`
		Value  []json.RawMessage   `json:"value"`
		Values [][]json.RawMessage `json:"values"`
	}
	if err := json.Unmarshal(raw, &vectorSample); err == nil {
		label := seriesLabel(vectorSample.Metric)
		if len(vectorSample.Values) > 0 {
			values := make([]float64, 0, len(vectorSample.Values))
			start := math.Inf(1)
			end := math.Inf(-1)
			for _, sample := range vectorSample.Values {
				if len(sample) < 2 {
					continue
				}
				ts, okTs := parsePromTimestamp(sample[0])
				value, okVal := parsePromValue(sample[1])
				if !okTs || !okVal {
					continue
				}
				values = append(values, value)
				if ts < start {
					start = ts
				}
				if ts > end {
					end = ts
				}
			}
			if len(values) > 0 {
				return label, values, start, end, true, true
			}
		}
		if len(vectorSample.Value) >= 2 {
			value, ok := parsePromValue(vectorSample.Value[1])
			if ok {
				return label, []float64{value}, 0, 0, false, true
			}
		}
	}
	var scalarSample []json.RawMessage
	if err := json.Unmarshal(raw, &scalarSample); err == nil && len(scalarSample) >= 2 {
		value, ok := parsePromValue(scalarSample[1])
		if ok {
			return "scalar", []float64{value}, 0, 0, false, true
		}
	}
	return "", nil, 0, 0, false, false
}

func parsePromTimestamp(raw json.RawMessage) (float64, bool) {
	var numberValue float64
	if err := json.Unmarshal(raw, &numberValue); err == nil {
		return numberValue, true
	}
	var stringValue string
	if err := json.Unmarshal(raw, &stringValue); err == nil {
		value, parseErr := strconv.ParseFloat(stringValue, 64)
		if parseErr == nil {
			return value, true
		}
	}
	return 0, false
}

func parsePromValue(raw json.RawMessage) (float64, bool) {
	var stringValue string
	if err := json.Unmarshal(raw, &stringValue); err == nil {
		value, parseErr := strconv.ParseFloat(stringValue, 64)
		if parseErr == nil {
			return value, true
		}
	}
	var numberValue float64
	if err := json.Unmarshal(raw, &numberValue); err == nil {
		return numberValue, true
	}
	return 0, false
}

func seriesLabel(metric map[string]string) string {
	if len(metric) == 0 {
		return "series"
	}
	if instance := strings.TrimSpace(metric["instance"]); instance != "" {
		if job := strings.TrimSpace(metric["job"]); job != "" {
			return job + "@" + instance
		}
		return instance
	}
	if name := strings.TrimSpace(metric["__name__"]); name != "" {
		return name
	}

	keys := make([]string, 0, len(metric))
	for key := range metric {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+metric[key])
	}
	return strings.Join(parts, ",")
}

func (m QueryModel) visibleSeries() []string {
	if len(m.seriesOrder) == 0 {
		return nil
	}
	if m.singleSeries {
		index := m.selectedSeries
		if index < 0 {
			index = 0
		}
		if index >= len(m.seriesOrder) {
			index = len(m.seriesOrder) - 1
		}
		return []string{m.seriesOrder[index]}
	}
	return m.seriesOrder
}

func (m QueryModel) seriesIndex(key string) int {
	for index, seriesKey := range m.seriesOrder {
		if key == seriesKey {
			return index
		}
	}
	return 0
}

func seriesColor(index int) asciigraph.AnsiColor {
	palette := []asciigraph.AnsiColor{
		asciigraph.Cyan,
		asciigraph.Magenta,
		asciigraph.Green,
		asciigraph.Yellow,
		asciigraph.Blue,
		asciigraph.Red,
		asciigraph.OrangeRed,
		asciigraph.LightSkyBlue,
	}
	return palette[index%len(palette)]
}

func formatUnixXAxis(value, spanSeconds float64) string {
	if spanSeconds <= 0 {
		return fmt.Sprintf("%.0f", value)
	}
	timestamp := time.Unix(int64(value), 0)
	if spanSeconds > 48*3600 {
		return timestamp.Format("01-02")
	}
	return timestamp.Format("15:04")
}

func trimLabel(value string, maxLen int) string {
	if len(value) <= maxLen {
		return value
	}
	if maxLen <= 3 {
		return value[:maxLen]
	}
	return value[:maxLen-3] + "..."
}
