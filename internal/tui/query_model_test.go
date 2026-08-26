package tui

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"vmcli/internal/api"
)

func TestNewQueryModel(t *testing.T) {
	result := api.QueryResult{Status: "success"}
	result.Data.ResultType = "vector"
	result.Data.Result = []json.RawMessage{
		json.RawMessage(`{"metric":{"__name__":"up"},"value":[12345,"1"]}`),
	}

	model := NewQuery(func(context.Context) (api.QueryResult, error) { return result, nil }, "up", "http://vmselect:8481", "instant", result, time.Second)
	if model.query != "up" {
		t.Fatalf("unexpected query: %s", model.query)
	}
	view := model.View()
	if !strings.Contains(view, "PromQL Query") {
		t.Fatalf("unexpected view: %s", view)
	}
	if !strings.Contains(view, "┼") {
		t.Fatalf("expected graph axes, got: %s", view)
	}
	if !strings.Contains(view, "Range:    instant") {
		t.Fatalf("expected range label, got: %s", view)
	}
}

func TestExtractSeriesTimelines(t *testing.T) {
	result := api.QueryResult{}
	result.Data.ResultType = "matrix"
	result.Data.Result = []json.RawMessage{
		json.RawMessage(`{"metric":{"job":"a","instance":"host-a:9100"},"values":[[12345,"1.5"],[12346,"1.6"]]}`),
		json.RawMessage(`{"metric":{"job":"b","instance":"host-b:9100"},"values":[[12345,"2.5"],[12346,"2.7"]]}`),
	}

	samples, start, end, hasTime := extractSeriesTimelines(result)
	if len(samples) != 2 {
		t.Fatalf("expected 2 samples, got %d", len(samples))
	}
	if !hasTime {
		t.Fatal("expected time range")
	}
	if start != 12345 || end != 12346 {
		t.Fatalf("unexpected range: %v -> %v", start, end)
	}
	if len(samples["a@host-a:9100"]) != 2 || samples["a@host-a:9100"][1] != 1.6 {
		t.Fatalf("unexpected sample for host-a: %+v", samples)
	}
	if len(samples["b@host-b:9100"]) != 2 || samples["b@host-b:9100"][1] != 2.7 {
		t.Fatalf("unexpected sample for host-b: %+v", samples)
	}
}

func TestSingleSeriesSelection(t *testing.T) {
	result := api.QueryResult{}
	result.Data.ResultType = "vector"
	result.Data.Result = []json.RawMessage{
		json.RawMessage(`{"metric":{"job":"a","instance":"host-a:9100"},"value":[12345,"1.5"]}`),
		json.RawMessage(`{"metric":{"job":"b","instance":"host-b:9100"},"value":[12345,"2.5"]}`),
	}

	model := NewQuery(func(context.Context) (api.QueryResult, error) { return result, nil }, "up", "http://vmselect:8481", "instant", result, time.Second)
	model.singleSeries = true
	model.selectedSeries = 1
	view := model.View()
	if !strings.Contains(view, "Mode:     single series (b@host-b:9100)") {
		t.Fatalf("expected selected series mode, got: %s", view)
	}
}
