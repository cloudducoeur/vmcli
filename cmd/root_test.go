package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"vmcli/internal/api"
	"vmcli/internal/config"

	"github.com/spf13/cobra"
)

func TestClusterUsesHTTPS(t *testing.T) {
	t.Run("returns false when all endpoints are http", func(t *testing.T) {
		cluster := config.Cluster{
			Vminsert:  []string{"http://insert:8480"},
			Vmselect:  []string{"http://select:8481"},
			Vmstorage: []string{"http://storage:8482"},
		}

		if clusterUsesHTTPS(cluster) {
			t.Fatal("expected false, got true")
		}
	})

	t.Run("returns true when at least one endpoint is https", func(t *testing.T) {
		cluster := config.Cluster{
			Vminsert:  []string{"http://insert:8480"},
			Vmselect:  []string{"https://select:8481"},
			Vmstorage: []string{"http://storage:8482"},
		}

		if !clusterUsesHTTPS(cluster) {
			t.Fatal("expected true, got false")
		}
	})

	t.Run("returns true when vmalert endpoint is https", func(t *testing.T) {
		cluster := config.Cluster{
			Vminsert:  []string{"http://insert:8480"},
			Vmselect:  []string{"http://select:8481"},
			Vmstorage: []string{"http://storage:8482"},
			Vmalert:   []string{"https://vmalert:8880"},
		}

		if !clusterUsesHTTPS(cluster) {
			t.Fatal("expected true, got false")
		}
	})
}

func TestQueryCommandFlags(t *testing.T) {
	command := queryCommand()
	if command.Flags().Lookup("raw") == nil {
		t.Fatal("expected raw flag")
	}
	if command.Flags().Lookup("refresh") == nil {
		t.Fatal("expected refresh flag")
	}
	if command.Flags().Lookup("last") == nil {
		t.Fatal("expected last flag")
	}
	if command.Flags().Lookup("from") == nil {
		t.Fatal("expected from flag")
	}
	if command.Flags().Lookup("to") == nil {
		t.Fatal("expected to flag")
	}
	if command.Flags().Lookup("step") == nil {
		t.Fatal("expected step flag")
	}
}

func TestRulesCommandFlags(t *testing.T) {
	command := rulesCommand()
	if command.Flags().Lookup("group") == nil {
		t.Fatal("expected group flag on rules command")
	}
	if command.Flags().Lookup("name") == nil {
		t.Fatal("expected name flag on rules command")
	}
	if command.Flags().Lookup("raw") == nil {
		t.Fatal("expected raw flag on rules command")
	}
}

func TestAlertsCommandFlags(t *testing.T) {
	command := alertsCommand()
	if command.Flags().Lookup("name") != nil || command.Flags().Lookup("raw") != nil {
		t.Fatal("did not expect name/raw flags on alerts command")
	}
}

func TestParseQueryTime(t *testing.T) {
	parsed, err := parseQueryTime("2026-08-26T11:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Year() != 2026 {
		t.Fatalf("unexpected year: %d", parsed.Year())
	}
}

func TestDeriveQueryStep(t *testing.T) {
	step := deriveQueryStep(5 * time.Minute)
	if step <= 0 {
		t.Fatalf("unexpected step: %s", step)
	}
}

func TestShouldRawAutoRefresh(t *testing.T) {
	command := queryCommand()
	if err := command.Flags().Set("refresh", "2s"); err != nil {
		t.Fatal(err)
	}
	refresh, err := command.Flags().GetDuration("refresh")
	if err != nil {
		t.Fatal(err)
	}
	if !shouldRawAutoRefresh(command, refresh) {
		t.Fatal("expected raw auto refresh to be enabled when refresh is specified")
	}

	withoutRefresh := queryCommand()
	if shouldRawAutoRefresh(withoutRefresh, 2*time.Second) {
		t.Fatal("expected raw auto refresh to be disabled when refresh is not specified")
	}

	zeroRefresh := queryCommand()
	if err := zeroRefresh.Flags().Set("refresh", "0s"); err != nil {
		t.Fatal(err)
	}
	if shouldRawAutoRefresh(zeroRefresh, 0) {
		t.Fatal("expected raw auto refresh to be disabled when refresh is 0")
	}
}

func TestRootWithoutArgsShowsHelp(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{})

	_, err := root.ExecuteC()
	if err != nil {
		t.Fatal(err)
	}
	helpText := stdout.String() + stderr.String()
	if !strings.Contains(helpText, "Available Commands") {
		t.Fatalf("expected help output, got: %s", helpText)
	}
	if !strings.Contains(helpText, "tui") {
		t.Fatalf("expected tui command in help, got: %s", helpText)
	}
	if !strings.Contains(helpText, "rules") {
		t.Fatalf("expected rules command in help, got: %s", helpText)
	}
	if !strings.Contains(helpText, "alerts") {
		t.Fatalf("expected alerts command in help, got: %s", helpText)
	}
	if !strings.Contains(helpText, "notifiers") {
		t.Fatalf("expected notifiers command in help, got: %s", helpText)
	}
	if !strings.Contains(helpText, "Examples") {
		t.Fatalf("expected examples section in root help, got: %s", helpText)
	}
	if !strings.Contains(helpText, "\n  vmcli\n") {
		t.Fatalf("expected indented examples, got: %s", helpText)
	}
}

func TestCommandsExposeShortAndExamples(t *testing.T) {
	commands := []*cobra.Command{
		nodesCommand(),
		healthCommand(),
		queryCommand(),
		rulesCommand(),
		alertsCommand(),
		notifiersCommand(),
		tuiCommand(),
		versionCommand(),
	}
	for _, command := range commands {
		if strings.TrimSpace(command.Short) == "" {
			t.Fatalf("expected short help for %s", command.Name())
		}
		if strings.TrimSpace(command.Example) == "" {
			t.Fatalf("expected example help for %s", command.Name())
		}
		if !command.SilenceUsage {
			t.Fatalf("expected SilenceUsage for %s", command.Name())
		}
		if !command.SilenceErrors {
			t.Fatalf("expected SilenceErrors for %s", command.Name())
		}
	}
}

func TestRootSilencesUsageAndErrors(t *testing.T) {
	if !root.SilenceUsage {
		t.Fatal("expected root.SilenceUsage to be true")
	}
	if !root.SilenceErrors {
		t.Fatal("expected root.SilenceErrors to be true")
	}
}

func TestPrintOutputToQueryTable(t *testing.T) {
	prevOutput := output
	output = "table"
	t.Cleanup(func() { output = prevOutput })

	result := api.QueryResult{Status: "success"}
	result.Data.ResultType = "vector"
	result.Data.Result = []json.RawMessage{
		json.RawMessage(`{"metric":{"job":"consul"},"value":[1,"0.42"]}`),
	}

	var buf bytes.Buffer
	if err := printOutputTo(&buf, result); err != nil {
		t.Fatal(err)
	}
	text := buf.String()
	if !strings.Contains(text, "status: success") {
		t.Fatalf("unexpected output: %s", text)
	}
	if !strings.Contains(text, `"job":"consul"`) {
		t.Fatalf("unexpected output: %s", text)
	}
}

func TestPrintOutputToVmalertRulesTable(t *testing.T) {
	prevOutput := output
	output = "table"
	t.Cleanup(func() { output = prevOutput })

	result := api.VmalertRulesResponse{Status: "success"}
	result.Data.Groups = []api.VmalertRuleGroup{
		{
			Name: "group-a",
			Rules: []api.VmalertRule{
				{Name: "RuleA", Type: "alerting", State: "inactive", Health: "ok"},
			},
		},
	}

	var buf bytes.Buffer
	if err := printOutputTo(&buf, result); err != nil {
		t.Fatal(err)
	}
	text := buf.String()
	if !strings.Contains(text, "GROUP") || !strings.Contains(text, "RuleA") {
		t.Fatalf("unexpected output: %s", text)
	}
}

func TestPrintOutputToVmalertRuleDetailTable(t *testing.T) {
	prevOutput := output
	output = "table"
	t.Cleanup(func() { output = prevOutput })

	detail := vmalertRuleDetail{
		Group: "group-a",
		Rule: api.VmalertRule{
			Name:           "RuleA",
			Type:           "alerting",
			State:          "inactive",
			Health:         "ok",
			Query:          "up == 0",
			Duration:       600,
			Labels:         map[string]string{"severity": "critical"},
			Annotations:    map[string]string{"summary": "Rule summary"},
			LastEvaluation: "2026-08-26T11:00:00Z",
		},
	}

	var buf bytes.Buffer
	if err := printOutputTo(&buf, detail); err != nil {
		t.Fatal(err)
	}
	text := buf.String()
	if !strings.Contains(text, "FIELD") || !strings.Contains(text, "group-a") || !strings.Contains(text, "up == 0") {
		t.Fatalf("unexpected output: %s", text)
	}
}

func TestPrintRawYAML(t *testing.T) {
	var buf bytes.Buffer
	value := vmalertRulesYAML{
		Groups: []vmalertRuleGroupYAML{
			{
				Name: "node_alerts",
				Rules: []vmalertRuleYAML{
					{
						Alert: "NodeExporterDown",
						Expr:  "rudder_up == 0",
						For:   "10m",
						Labels: map[string]string{
							"severity": "critical",
						},
						Annotations: map[string]string{
							"summary": "Node exporter is down",
						},
					},
				},
			},
		},
	}
	if err := printRawYAML(&buf, value); err != nil {
		t.Fatal(err)
	}
	text := buf.String()
	if !strings.Contains(text, "groups:") ||
		!strings.Contains(text, "alert: NodeExporterDown") ||
		!strings.Contains(text, "expr: rudder_up == 0") ||
		!strings.Contains(text, "for: 10m") {
		t.Fatalf("unexpected raw yaml output: %s", text)
	}
}

func TestVmalertRuleToYAML(t *testing.T) {
	rule := api.VmalertRule{
		Name:        "NodeExporterDown",
		Type:        "alerting",
		Query:       "rudder_up == 0",
		Duration:    600,
		Labels:      map[string]string{"severity": "critical"},
		Annotations: map[string]string{"summary": "Node exporter is down"},
	}
	yamlRule := vmalertRuleToYAML(rule)
	if yamlRule.Alert != "NodeExporterDown" {
		t.Fatalf("unexpected alert name: %+v", yamlRule)
	}
	if yamlRule.Expr != "rudder_up == 0" {
		t.Fatalf("unexpected expr: %+v", yamlRule)
	}
	if yamlRule.For != "10m" {
		t.Fatalf("unexpected for value: %+v", yamlRule)
	}
}

func TestPrintOutputToVmalertAlertsTable(t *testing.T) {
	prevOutput := output
	output = "table"
	t.Cleanup(func() { output = prevOutput })

	result := api.VmalertAlertsResponse{Status: "success"}
	result.Data.Alerts = []api.VmalertAlert{
		{Name: "AlertA", GroupName: "node_alerts", State: "firing", Value: "1", ActiveAt: "2026-08-26T11:00:00Z"},
	}

	var buf bytes.Buffer
	if err := printOutputTo(&buf, result); err != nil {
		t.Fatal(err)
	}
	text := buf.String()
	if !strings.Contains(text, "GROUP") || !strings.Contains(text, "node_alerts") || !strings.Contains(text, "AlertA") {
		t.Fatalf("unexpected output: %s", text)
	}
}

func TestPrintOutputToVmalertNotifiersTable(t *testing.T) {
	prevOutput := output
	output = "table"
	t.Cleanup(func() { output = prevOutput })

	result := api.VmalertNotifiersResponse{Status: "success"}
	result.Data.Notifiers = []api.VmalertNotifier{
		{
			Kind: "static",
			Targets: []api.VmalertNotifierTarget{
				{Address: "http://localhost:9093/api/v2/alerts", LastError: ""},
			},
		},
	}

	var buf bytes.Buffer
	if err := printOutputTo(&buf, result); err != nil {
		t.Fatal(err)
	}
	text := buf.String()
	if !strings.Contains(text, "KIND") || !strings.Contains(text, "localhost:9093") {
		t.Fatalf("unexpected output: %s", text)
	}
}

func TestVmalertAlertName(t *testing.T) {
	alert := api.VmalertAlert{Name: "ByName", Labels: map[string]string{"alertname": "ByLabel"}}
	if got := vmalertAlertName(alert); got != "ByName" {
		t.Fatalf("unexpected alert name: %s", got)
	}
	alert = api.VmalertAlert{Labels: map[string]string{"alertname": "ByLabel"}}
	if got := vmalertAlertName(alert); got != "ByLabel" {
		t.Fatalf("unexpected alert name: %s", got)
	}
}

func TestFindVmalertRule(t *testing.T) {
	response := api.VmalertRulesResponse{}
	response.Data.Groups = []api.VmalertRuleGroup{
		{
			Name: "group-a",
			Rules: []api.VmalertRule{
				{Name: "RuleA"},
			},
		},
	}
	detail, found := findVmalertRule(response, "RuleA")
	if !found {
		t.Fatal("expected rule to be found")
	}
	if detail.Group != "group-a" || detail.Rule.Name != "RuleA" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
}

func TestFilterVmalertRulesByGroup(t *testing.T) {
	response := api.VmalertRulesResponse{Status: "success"}
	response.Data.Groups = []api.VmalertRuleGroup{
		{Name: "g1", Rules: []api.VmalertRule{{Name: "A1"}}},
		{Name: "g2", Rules: []api.VmalertRule{{Name: "A2"}}},
	}
	filtered := filterVmalertRulesByGroup(response, "g1")
	if len(filtered.Data.Groups) != 1 {
		t.Fatalf("unexpected filtered size: %d", len(filtered.Data.Groups))
	}
	if filtered.Data.Groups[0].Name != "g1" || filtered.Data.Groups[0].Rules[0].Name != "A1" {
		t.Fatalf("unexpected filtered data: %+v", filtered.Data.Groups)
	}
}
