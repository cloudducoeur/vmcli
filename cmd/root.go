package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"vmcli/internal/api"
	"vmcli/internal/cluster"
	"vmcli/internal/config"
	"vmcli/internal/models"
	"vmcli/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var configPath, clusterName, output string
var root = &cobra.Command{
	Use:           "vmcli",
	Short:         "Terminal supervision for VictoriaMetrics Cluster",
	Long:          "vmcli supervises VictoriaMetrics cluster components and vmalert endpoints from the terminal.",
	SilenceUsage:  true,
	SilenceErrors: true,
	Example: exampleBlock(
		"vmcli",
		"vmcli tui --cluster production",
		"vmcli nodes --output table",
		"vmcli query 'up'",
		"vmcli query --last 5m --step 10s 'sum(rate(process_cpu_seconds_total)) by (job)'",
		"vmcli rules",
		"vmcli alerts",
		"vmcli notifiers --output json",
	),
}

func init() {
	root.PersistentFlags().StringVar(&configPath, "config", "", "configuration file")
	root.PersistentFlags().StringVar(&clusterName, "cluster", "", "cluster name")
	root.PersistentFlags().StringVar(&output, "output", "table", "output format: table, json, yaml")
	root.AddCommand(nodesCommand(), healthCommand(), queryCommand(), rulesCommand(), alertsCommand(), notifiersCommand(), tuiCommand(), versionCommand())
}
func Execute() {
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func load() (config.File, config.Cluster, error) {
	file, err := config.Load(configPath)
	if err != nil {
		return file, config.Cluster{}, err
	}
	if clusterName == "" {
		for name := range file.Clusters {
			clusterName = name
			break
		}
	}
	selected, ok := file.Clusters[clusterName]
	if !ok {
		return file, selected, fmt.Errorf("cluster %q not found", clusterName)
	}
	return file, selected, nil
}

func clusterUsesHTTPS(c config.Cluster) bool {
	endpoints := append(append(append(append([]string{}, c.Vminsert...), c.Vmselect...), c.Vmstorage...), c.Vmalert...)
	for _, endpoint := range endpoints {
		parsed, err := url.Parse(strings.TrimSpace(endpoint))
		if err == nil && strings.EqualFold(parsed.Scheme, "https") {
			return true
		}
	}
	return false
}

func clientFor(c config.Cluster) (*api.Client, error) {
	tlsConfig := c.TLS
	if !clusterUsesHTTPS(c) {
		tlsConfig = config.TLS{}
	}
	return api.NewClient(tlsConfig, c.Auth)
}

func runTUI(cmd *cobra.Command, args []string) error {
	file, selected, err := load()
	if err != nil {
		return err
	}
	client, err := clientFor(selected)
	if err != nil {
		return err
	}
	program := tea.NewProgram(tui.New(client, selected, clusterName, file.RefreshInterval), tea.WithAltScreen())
	_, err = program.Run()
	return err
}

func tuiCommand() *cobra.Command {
	return &cobra.Command{
		Use:           "tui",
		Short:         "Run interactive TUI",
		SilenceUsage:  true,
		SilenceErrors: true,
		Example: exampleBlock(
			"vmcli tui",
			"vmcli tui --cluster production",
			"vmcli tui --config ~/.config/vmcli/config.yaml --cluster production",
		),
		RunE: runTUI,
	}
}

func nodesCommand() *cobra.Command {
	return &cobra.Command{Use: "nodes", Short: "List cluster node health", SilenceUsage: true, SilenceErrors: true, Example: exampleBlock(
		"vmcli nodes",
		"vmcli nodes --cluster production",
		"vmcli nodes --output json",
	), RunE: func(*cobra.Command, []string) error {
		file, selected, err := load()
		if err != nil {
			return err
		}
		client, err := clientFor(selected)
		if err != nil {
			return err
		}
		nodes := cluster.Check(context.Background(), client, cluster.Nodes(selected))
		return printOutput(nodes, file.RefreshInterval)
	}}
}

func rulesCommand() *cobra.Command {
	command := &cobra.Command{Use: "rules", Short: "List vmalert rules", SilenceUsage: true, SilenceErrors: true, Example: exampleBlock(
		"vmcli rules",
		"vmcli rules --cluster production",
		"vmcli rules --output yaml",
		"vmcli rules --group node_alerts",
		"vmcli rules --name NodeExporterDown",
		"vmcli rules --name NodeExporterDown --raw",
	), RunE: func(cmd *cobra.Command, args []string) error {
		_, selected, err := load()
		if err != nil {
			return err
		}
		client, err := clientFor(selected)
		if err != nil {
			return err
		}
		endpoint, err := vmalertEndpoint(selected)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
		defer cancel()
		result, err := client.VmalertRules(ctx, endpoint)
		if err != nil {
			return err
		}
		groupName, err := cmd.Flags().GetString("group")
		if err != nil {
			return err
		}
		if strings.TrimSpace(groupName) != "" {
			result = filterVmalertRulesByGroup(result, groupName)
			if len(result.Data.Groups) == 0 {
				return fmt.Errorf("group %q not found", groupName)
			}
		}
		ruleName, err := cmd.Flags().GetString("name")
		if err != nil {
			return err
		}
		raw, err := cmd.Flags().GetBool("raw")
		if err != nil {
			return err
		}
		if strings.TrimSpace(ruleName) != "" {
			detail, found := findVmalertRule(result, ruleName)
			if !found {
				return fmt.Errorf("rule %q not found", ruleName)
			}
			if raw {
				return printRawYAML(os.Stdout, vmalertRulesToYAML(api.VmalertRulesResponse{
					Data: struct {
						Groups []api.VmalertRuleGroup `json:"groups"`
					}{
						Groups: []api.VmalertRuleGroup{{
							Name:  detail.Group,
							Rules: []api.VmalertRule{detail.Rule},
						}},
					},
				}))
			}
			return printOutput(detail, 0)
		}
		if raw {
			return printRawYAML(os.Stdout, vmalertRulesToYAML(result))
		}
		return printOutput(result, 0)
	}}
	command.Flags().String("group", "", "filter rules by vmalert group name")
	command.Flags().String("name", "", "show details for a specific rule name")
	command.Flags().Bool("raw", false, "print raw YAML payload")
	return command
}

func alertsCommand() *cobra.Command {
	return &cobra.Command{Use: "alerts", Short: "List vmalert alerts", SilenceUsage: true, SilenceErrors: true, Example: exampleBlock(
		"vmcli alerts",
		"vmcli alerts --cluster production",
		"vmcli alerts --output json",
	), RunE: func(cmd *cobra.Command, args []string) error {
		_, selected, err := load()
		if err != nil {
			return err
		}
		client, err := clientFor(selected)
		if err != nil {
			return err
		}
		endpoint, err := vmalertEndpoint(selected)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
		defer cancel()
		result, err := client.VmalertAlerts(ctx, endpoint)
		if err != nil {
			return err
		}
		return printOutput(result, 0)
	}}
}

func notifiersCommand() *cobra.Command {
	return &cobra.Command{Use: "notifiers", Short: "List vmalert notifiers", SilenceUsage: true, SilenceErrors: true, Example: exampleBlock(
		"vmcli notifiers",
		"vmcli notifiers --cluster production",
		"vmcli notifiers --output yaml",
	), RunE: func(cmd *cobra.Command, args []string) error {
		_, selected, err := load()
		if err != nil {
			return err
		}
		client, err := clientFor(selected)
		if err != nil {
			return err
		}
		endpoint, err := vmalertEndpoint(selected)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
		defer cancel()
		result, err := client.VmalertNotifiers(ctx, endpoint)
		if err != nil {
			return err
		}
		return printOutput(result, 0)
	}}
}

func healthCommand() *cobra.Command {
	return &cobra.Command{
		Use:           "health",
		Short:         "Alias for nodes health check",
		SilenceUsage:  true,
		SilenceErrors: true,
		Example: exampleBlock(
			"vmcli health",
			"vmcli health --cluster production",
			"vmcli health --output json",
		),
		RunE: func(*cobra.Command, []string) error { return nodesCommand().RunE(nil, nil) },
	}
}
func queryCommand() *cobra.Command {
	command := &cobra.Command{Use: "query PROMQL", Short: "Run a PromQL query", SilenceUsage: true, SilenceErrors: true, Example: exampleBlock(
		"vmcli query 'up'",
		"vmcli query --cluster production 'sum(rate(process_cpu_seconds_total)) by (job)'",
		"vmcli query --last 5m --step 10s 'up'",
		"vmcli query --from '2026-08-26T09:00:00Z' --to '2026-08-26T10:00:00Z' 'up'",
		"vmcli query --raw --refresh 2s 'sum(rate(process_cpu_seconds_total)) by (job)'",
	), Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		file, selected, err := load()
		if err != nil {
			return err
		}

		client, err := clientFor(selected)
		if err != nil {
			return err
		}
		endpoint := ""
		if len(selected.Vmselect) > 0 {
			endpoint = selected.Vmselect[0]
		} else {
			return fmt.Errorf("cluster has no vmselect endpoint")
		}
		fetch, rangeLabel, err := buildQueryFetcher(cmd, client, endpoint, selected.Tenant.AccountID, selected.Tenant.ProjectID, args[0])
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
		defer cancel()
		result, err := fetch(ctx)
		if err != nil {
			return err
		}
		refreshInterval, err := cmd.Flags().GetDuration("refresh")
		if err != nil {
			return err
		}
		if refreshInterval < 0 {
			return fmt.Errorf("refresh must be greater than or equal to 0")
		}
		raw, err := cmd.Flags().GetBool("raw")
		if err != nil {
			return err
		}
		if raw {
			if shouldRawAutoRefresh(cmd, refreshInterval) {
				return runRawQueryLoop(cmd.Context(), fetch, result, refreshInterval)
			}
			return printOutput(result, 0)
		}
		if !cmd.Flags().Changed("refresh") {
			refreshInterval = file.RefreshInterval
		}
		program := tea.NewProgram(tui.NewQuery(fetch, args[0], endpoint, rangeLabel, result, refreshInterval))
		_, err = program.Run()
		return err
	}}
	command.Flags().Bool("raw", false, "print raw query output instead of query TUI")
	command.Flags().Duration("refresh", 0, "auto-refresh interval for query TUI (0 disables)")
	command.Flags().Duration("last", 0, "query range over the last duration (for example 5m)")
	command.Flags().String("from", "", "query range start time (RFC3339 or 2006-01-02[ 15:04[:05]])")
	command.Flags().String("to", "", "query range end time (RFC3339 or 2006-01-02[ 15:04[:05]])")
	command.Flags().Duration("step", 0, "query range step interval (auto when omitted)")
	return command
}

func versionCommand() *cobra.Command {
	return &cobra.Command{
		Use:           "version",
		Short:         "Print vmcli version",
		SilenceUsage:  true,
		SilenceErrors: true,
		Example:       exampleBlock("vmcli version"),
		Run: func(*cobra.Command, []string) {
			fmt.Println("vmcli 0.1.0")
		},
	}
}

func exampleBlock(lines ...string) string {
	if len(lines) == 0 {
		return ""
	}
	for index, line := range lines {
		lines[index] = "  " + line
	}
	return strings.Join(lines, "\n")
}

func buildQueryFetcher(cmd *cobra.Command, client api.VictoriaMetricsClient, endpoint string, accountID, projectID int, query string) (func(context.Context) (api.QueryResult, error), string, error) {
	last, err := cmd.Flags().GetDuration("last")
	if err != nil {
		return nil, "", err
	}
	fromRaw, err := cmd.Flags().GetString("from")
	if err != nil {
		return nil, "", err
	}
	toRaw, err := cmd.Flags().GetString("to")
	if err != nil {
		return nil, "", err
	}
	step, err := cmd.Flags().GetDuration("step")
	if err != nil {
		return nil, "", err
	}
	if last < 0 {
		return nil, "", fmt.Errorf("last must be greater than or equal to 0")
	}
	if step < 0 {
		return nil, "", fmt.Errorf("step must be greater than or equal to 0")
	}

	hasLast := cmd.Flags().Changed("last") && last > 0
	hasFrom := strings.TrimSpace(fromRaw) != ""
	hasTo := strings.TrimSpace(toRaw) != ""
	hasRange := hasLast || hasFrom || hasTo

	if hasLast && (hasFrom || hasTo) {
		return nil, "", fmt.Errorf("last cannot be combined with from/to")
	}
	if hasFrom != hasTo {
		return nil, "", fmt.Errorf("from and to must be provided together")
	}
	if !hasRange && cmd.Flags().Changed("step") {
		return nil, "", fmt.Errorf("step is only valid with last or from/to")
	}

	if hasLast {
		rangeStep := step
		if rangeStep == 0 {
			rangeStep = deriveQueryStep(last)
		}
		return func(ctx context.Context) (api.QueryResult, error) {
			end := time.Now()
			start := end.Add(-last)
			return client.QueryRange(ctx, endpoint, accountID, projectID, query, start, end, rangeStep)
		}, fmt.Sprintf("last %s (step %s)", last, rangeStep), nil
	}

	if hasFrom && hasTo {
		start, err := parseQueryTime(fromRaw)
		if err != nil {
			return nil, "", fmt.Errorf("invalid from value: %w", err)
		}
		end, err := parseQueryTime(toRaw)
		if err != nil {
			return nil, "", fmt.Errorf("invalid to value: %w", err)
		}
		if !end.After(start) {
			return nil, "", fmt.Errorf("to must be after from")
		}
		rangeStep := step
		if rangeStep == 0 {
			rangeStep = deriveQueryStep(end.Sub(start))
		}
		return func(ctx context.Context) (api.QueryResult, error) {
			return client.QueryRange(ctx, endpoint, accountID, projectID, query, start, end, rangeStep)
		}, fmt.Sprintf("%s -> %s (step %s)", start.Format(time.RFC3339), end.Format(time.RFC3339), rangeStep), nil
	}

	return func(ctx context.Context) (api.QueryResult, error) {
		return client.Query(ctx, endpoint, accountID, projectID, query)
	}, "instant", nil
}

func parseQueryTime(value string) (time.Time, error) {
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
	}
	trimmed := strings.TrimSpace(value)
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			return parsed, nil
		}
		if parsed, err := time.ParseInLocation(layout, trimmed, time.Local); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported time format %q", value)
}

func deriveQueryStep(duration time.Duration) time.Duration {
	if duration <= 0 {
		return time.Second
	}
	step := duration / 120
	if step < time.Second {
		return time.Second
	}
	if step > 5*time.Minute {
		return 5 * time.Minute
	}
	return step
}

func shouldRawAutoRefresh(cmd *cobra.Command, refreshInterval time.Duration) bool {
	return cmd.Flags().Changed("refresh") && refreshInterval > 0
}

func runRawQueryLoop(ctx context.Context, fetch func(context.Context) (api.QueryResult, error), first api.QueryResult, refreshInterval time.Duration) error {
	renderInPlace := func(result api.QueryResult) error {
		var buffer bytes.Buffer
		if err := printOutputTo(&buffer, result); err != nil {
			return err
		}
		frame := buffer.String()
		if !strings.HasSuffix(frame, "\n") {
			frame += "\n"
		}
		if _, err := fmt.Fprint(os.Stdout, "\x1b[H\x1b[2J"); err != nil {
			return err
		}
		_, err := fmt.Fprint(os.Stdout, frame)
		return err
	}
	if err := renderInPlace(first); err != nil {
		return err
	}
	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.Canceled) {
				return nil
			}
			return ctx.Err()
		case <-ticker.C:
			queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			result, err := fetch(queryCtx)
			cancel()
			if err != nil {
				return err
			}
			if err := renderInPlace(result); err != nil {
				return err
			}
		}
	}
}

func vmalertEndpoint(selected config.Cluster) (string, error) {
	if len(selected.Vmalert) == 0 {
		return "", fmt.Errorf("cluster has no vmalert endpoint")
	}
	return selected.Vmalert[0], nil
}

func printOutput(value any, _ time.Duration) error {
	return printOutputTo(os.Stdout, value)
}

func printRawYAML(writer io.Writer, value any) error {
	data, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
}

func printOutputTo(writer io.Writer, value any) error {
	switch output {
	case "json":
		encoder := json.NewEncoder(writer)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	case "yaml":
		data, err := yaml.Marshal(value)
		if err != nil {
			return err
		}
		_, err = writer.Write(data)
		return err
	default:
		switch typed := value.(type) {
		case []models.Node:
			rows := make([][]string, 0, len(typed))
			for _, node := range typed {
				rows = append(rows, []string{
					string(node.Component),
					nodeDisplayName(node.URL),
					nodeHost(node.URL),
					renderNodeStatus(node.Status),
					nodeValueOrDash(node.Version),
					nodeValueOrDash(node.Uptime),
					nodeMetricsSummary(node.Metrics),
					formatCheckedAt(node.CheckedAt),
				})
			}
			_, err := io.WriteString(writer, renderStyledTable(
				[]string{"COMPONENT", "NAME", "HOST", "STATUS", "VERSION", "UPTIME", "METRICS", "CHECKED"},
				rows,
			))
			return err
		case api.QueryResult:
			fmt.Fprintf(writer, "status: %s\nresult type: %s\n", typed.Status, typed.Data.ResultType)
			for _, result := range typed.Data.Result {
				fmt.Fprintln(writer, string(result))
			}
		case api.VmalertRulesResponse:
			rows := make([][]string, 0)
			for _, group := range typed.Data.Groups {
				for _, rule := range group.Rules {
					rows = append(rows, []string{group.Name, rule.Name, rule.Type, rule.State, rule.Health})
				}
			}
			_, err := io.WriteString(writer, renderStyledTable(
				[]string{"GROUP", "RULE", "TYPE", "STATE", "HEALTH"},
				rows,
			))
			return err
		case vmalertRuleDetail:
			rows := [][]string{
				{"group", typed.Group},
				{"name", typed.Rule.Name},
				{"type", typed.Rule.Type},
				{"state", typed.Rule.State},
				{"health", typed.Rule.Health},
				{"query", typed.Rule.Query},
				{"duration", fmt.Sprintf("%ds", typed.Rule.Duration)},
				{"keep_firing_for", fmt.Sprintf("%ds", typed.Rule.KeepFiringFor)},
				{"last_evaluation", typed.Rule.LastEvaluation},
			}
			if typed.Rule.LastError != "" {
				rows = append(rows, []string{"last_error", typed.Rule.LastError})
			}
			if len(typed.Rule.Labels) > 0 {
				rows = append(rows, []string{"labels", formatMap(typed.Rule.Labels)})
			}
			if len(typed.Rule.Annotations) > 0 {
				rows = append(rows, []string{"annotations", formatMap(typed.Rule.Annotations)})
			}
			_, err := io.WriteString(writer, renderStyledTable(
				[]string{"FIELD", "VALUE"},
				rows,
			))
			return err
		case api.VmalertAlertsResponse:
			rows := make([][]string, 0, len(typed.Data.Alerts))
			for _, alert := range typed.Data.Alerts {
				rows = append(rows, []string{vmalertAlertGroup(alert), vmalertAlertName(alert), alert.State, alert.Value, alert.ActiveAt})
			}
			_, err := io.WriteString(writer, renderStyledTable(
				[]string{"GROUP", "ALERT", "STATE", "VALUE", "ACTIVE_AT"},
				rows,
			))
			return err
		case api.VmalertNotifiersResponse:
			rows := make([][]string, 0)
			for _, notifier := range typed.Data.Notifiers {
				for _, target := range notifier.Targets {
					rows = append(rows, []string{notifier.Kind, target.Address, target.LastError})
				}
			}
			_, err := io.WriteString(writer, renderStyledTable(
				[]string{"KIND", "ADDRESS", "LAST_ERROR"},
				rows,
			))
			return err
		default:
			return fmt.Errorf("table output is not supported for %T", value)
		}
		return nil
	}
}

func vmalertAlertName(alert api.VmalertAlert) string {
	if strings.TrimSpace(alert.Name) != "" {
		return alert.Name
	}
	if alertName, ok := alert.Labels["alertname"]; ok && strings.TrimSpace(alertName) != "" {
		return alertName
	}
	return "-"
}

func vmalertAlertGroup(alert api.VmalertAlert) string {
	if strings.TrimSpace(alert.GroupName) != "" {
		return alert.GroupName
	}
	if groupName, ok := alert.Labels["group_name"]; ok && strings.TrimSpace(groupName) != "" {
		return groupName
	}
	if groupName, ok := alert.Labels["group"]; ok && strings.TrimSpace(groupName) != "" {
		return groupName
	}
	return "-"
}

type vmalertRuleDetail struct {
	Group string          `json:"group"`
	Rule  api.VmalertRule `json:"rule"`
}

type vmalertRulesYAML struct {
	Groups []vmalertRuleGroupYAML `yaml:"groups"`
}

type vmalertRuleGroupYAML struct {
	Name  string            `yaml:"name"`
	Rules []vmalertRuleYAML `yaml:"rules"`
}

type vmalertRuleYAML struct {
	Alert       string            `yaml:"alert,omitempty"`
	Record      string            `yaml:"record,omitempty"`
	Expr        string            `yaml:"expr"`
	For         string            `yaml:"for,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
}

func findVmalertRule(response api.VmalertRulesResponse, name string) (vmalertRuleDetail, bool) {
	target := strings.TrimSpace(name)
	for _, group := range response.Data.Groups {
		for _, rule := range group.Rules {
			if rule.Name == target {
				return vmalertRuleDetail{Group: group.Name, Rule: rule}, true
			}
		}
	}
	return vmalertRuleDetail{}, false
}

func filterVmalertRulesByGroup(response api.VmalertRulesResponse, name string) api.VmalertRulesResponse {
	target := strings.TrimSpace(name)
	filtered := api.VmalertRulesResponse{Status: response.Status}
	filtered.Data.Groups = make([]api.VmalertRuleGroup, 0)
	for _, group := range response.Data.Groups {
		if group.Name == target {
			filtered.Data.Groups = append(filtered.Data.Groups, group)
		}
	}
	return filtered
}

func formatMap(values map[string]string) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", key, values[key]))
	}
	return strings.Join(parts, ", ")
}

func vmalertRulesToYAML(response api.VmalertRulesResponse) vmalertRulesYAML {
	groups := make([]vmalertRuleGroupYAML, 0, len(response.Data.Groups))
	for _, group := range response.Data.Groups {
		rules := make([]vmalertRuleYAML, 0, len(group.Rules))
		for _, rule := range group.Rules {
			rules = append(rules, vmalertRuleToYAML(rule))
		}
		groups = append(groups, vmalertRuleGroupYAML{Name: group.Name, Rules: rules})
	}
	return vmalertRulesYAML{Groups: groups}
}

func vmalertRuleToYAML(rule api.VmalertRule) vmalertRuleYAML {
	result := vmalertRuleYAML{
		Expr:        rule.Query,
		Labels:      rule.Labels,
		Annotations: rule.Annotations,
	}
	if strings.EqualFold(rule.Type, "recording") {
		result.Record = rule.Name
	} else {
		result.Alert = rule.Name
	}
	if rule.Duration > 0 {
		result.For = formatRuleFor(rule.Duration)
	}
	return result
}

func formatRuleFor(seconds int) string {
	duration := time.Duration(seconds) * time.Second
	total := int(duration / time.Second)
	hours := total / 3600
	minutes := (total % 3600) / 60
	secs := total % 60

	var b strings.Builder
	if hours > 0 {
		fmt.Fprintf(&b, "%dh", hours)
	}
	if minutes > 0 {
		fmt.Fprintf(&b, "%dm", minutes)
	}
	if secs > 0 || b.Len() == 0 {
		fmt.Fprintf(&b, "%ds", secs)
	}
	return b.String()
}

func renderStyledTable(headers []string, rows [][]string) string {
	widths := make([]int, len(headers))
	for index, header := range headers {
		width := lipgloss.Width(header)
		for _, row := range rows {
			if index >= len(row) {
				continue
			}
			cellWidth := lipgloss.Width(row[index])
			if cellWidth > width {
				width = cellWidth
			}
		}
		widths[index] = width
	}

	pad := func(value string, width int) string {
		if gap := width - lipgloss.Width(value); gap > 0 {
			return value + strings.Repeat(" ", gap)
		}
		return value
	}

	borderLine := func(left, middle, right string) string {
		parts := make([]string, len(widths))
		for index, width := range widths {
			parts[index] = strings.Repeat("─", width+2)
		}
		return left + strings.Join(parts, middle) + right
	}

	rowLine := func(values []string) string {
		cells := make([]string, len(widths))
		for index, width := range widths {
			value := ""
			if index < len(values) {
				value = values[index]
			}
			cells[index] = " " + pad(value, width) + " "
		}
		return "│" + strings.Join(cells, "│") + "│"
	}

	var b strings.Builder
	b.WriteString(borderLine("╭", "┬", "╮"))
	b.WriteByte('\n')
	b.WriteString(rowLine(headers))
	b.WriteByte('\n')
	b.WriteString(borderLine("├", "┼", "┤"))
	b.WriteByte('\n')
	for _, row := range rows {
		b.WriteString(rowLine(row))
		b.WriteByte('\n')
	}
	b.WriteString(borderLine("╰", "┴", "╯"))
	b.WriteByte('\n')
	return b.String()
}

func renderNodeStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "UP":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#4ADE80")).Render("● UP")
	case "DOWN":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#F87171")).Render("● DOWN")
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#A3A3A3")).Render("● " + status)
	}
}

func nodeDisplayName(endpoint string) string {
	host := nodeHost(endpoint)
	if host == "" {
		return "—"
	}
	if index := strings.Index(host, "."); index >= 0 {
		return host[:index]
	}
	return host
}

func nodeHost(endpoint string) string {
	value := strings.TrimSpace(endpoint)
	value = strings.TrimPrefix(value, "http://")
	value = strings.TrimPrefix(value, "https://")
	if index := strings.Index(value, "/"); index >= 0 {
		value = value[:index]
	}
	if value == "" {
		return "—"
	}
	return value
}

func nodeValueOrDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "—"
	}
	return value
}

func nodeMetricsSummary(metrics map[string]float64) string {
	if len(metrics) == 0 {
		return "—"
	}
	keys := make([]string, 0, len(metrics))
	for key := range metrics {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	key := keys[0]
	return fmt.Sprintf("%s=%s", key, strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", metrics[key]), "0"), "."))
}

func formatCheckedAt(ts time.Time) string {
	if ts.IsZero() {
		return "—"
	}
	return ts.Format("15:04:05")
}
