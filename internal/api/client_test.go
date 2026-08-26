package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"vmcli/internal/config"
)

func TestClientHealthAndMetrics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/metrics":
			_, _ = w.Write([]byte("# HELP vm_rows  rows\nvm_rows 42\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := NewClient(config.TLS{}, config.Auth{})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Health(context.Background(), server.URL); err != nil {
		t.Fatal(err)
	}
	metrics, err := client.Metrics(context.Background(), server.URL)
	if err != nil || metrics["vm_rows"] != 42 {
		t.Fatalf("metrics=%v err=%v", metrics, err)
	}
}

func TestNewClientSkipsCAFileWhenInsecureSkipVerifyEnabled(t *testing.T) {
	client, err := NewClient(config.TLS{
		CAFile:             "/path/that/does/not/exist.pem",
		InsecureSkipVerify: true,
	}, config.Auth{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if client == nil {
		t.Fatal("expected client, got nil")
	}
}

func TestNewClientReadsCAFileWhenInsecureSkipVerifyDisabled(t *testing.T) {
	_, err := NewClient(config.TLS{
		CAFile:             "/path/that/does/not/exist.pem",
		InsecureSkipVerify: false,
	}, config.Auth{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "read CA file") {
		t.Fatalf("expected CA file error, got %v", err)
	}
}

func TestClientQueryRange(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/select/2/prometheus/api/v1/query_range" {
			http.NotFound(w, r)
			return
		}
		query := r.URL.Query()
		if query.Get("query") != "up" {
			t.Fatalf("unexpected query param: %s", query.Get("query"))
		}
		if query.Get("start") != "1700000000" {
			t.Fatalf("unexpected start param: %s", query.Get("start"))
		}
		if query.Get("end") != "1700000300" {
			t.Fatalf("unexpected end param: %s", query.Get("end"))
		}
		step, err := url.QueryUnescape(query.Get("step"))
		if err != nil {
			t.Fatal(err)
		}
		if step != "30s" {
			t.Fatalf("unexpected step param: %s", step)
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
	}))
	defer server.Close()

	client, err := NewClient(config.TLS{}, config.Auth{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.QueryRange(
		context.Background(),
		server.URL,
		1,
		2,
		"up",
		time.Unix(1700000000, 0),
		time.Unix(1700000300, 0),
		30*time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "success" {
		t.Fatalf("unexpected status: %s", result.Status)
	}
}

func TestClientVmalertRules(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/rules" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"groups":[{"name":"g1","rules":[{"name":"R1","state":"inactive","type":"alerting","health":"ok"}]}]}}`))
	}))
	defer server.Close()

	client, err := NewClient(config.TLS{}, config.Auth{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.VmalertRules(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "success" || len(result.Data.Groups) != 1 {
		t.Fatalf("unexpected response: %+v", result)
	}
}

func TestVmalertRulesURL(t *testing.T) {
	if got := vmalertRulesURL("https://vmalert.internal.rdcnet.org"); got != "https://vmalert.internal.rdcnet.org/api/v1/rules" {
		t.Fatalf("unexpected URL: %s", got)
	}
	if got := vmalertRulesURL("https://vmalert.internal.rdcnet.org/api/v1/rules"); got != "https://vmalert.internal.rdcnet.org/api/v1/rules" {
		t.Fatalf("unexpected URL: %s", got)
	}
}

func TestClientVmalertAlerts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/alerts" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"alerts":[{"name":"AlertA","state":"firing","value":"1"}]}}`))
	}))
	defer server.Close()

	client, err := NewClient(config.TLS{}, config.Auth{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.VmalertAlerts(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "success" || len(result.Data.Alerts) != 1 {
		t.Fatalf("unexpected response: %+v", result)
	}
}

func TestClientVmalertNotifiers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/notifiers" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"notifiers":[{"kind":"static","targets":[{"address":"http://localhost:9093/api/v2/alerts","labels":{},"lastError":""}]}]}}`))
	}))
	defer server.Close()

	client, err := NewClient(config.TLS{}, config.Auth{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.VmalertNotifiers(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "success" || len(result.Data.Notifiers) != 1 {
		t.Fatalf("unexpected response: %+v", result)
	}
}

func TestVmalertAlertsURL(t *testing.T) {
	if got := vmalertAlertsURL("https://vmalert.internal.rdcnet.org"); got != "https://vmalert.internal.rdcnet.org/api/v1/alerts" {
		t.Fatalf("unexpected URL: %s", got)
	}
	if got := vmalertAlertsURL("https://vmalert.internal.rdcnet.org/api/v1/alerts"); got != "https://vmalert.internal.rdcnet.org/api/v1/alerts" {
		t.Fatalf("unexpected URL: %s", got)
	}
}

func TestVmalertNotifiersURL(t *testing.T) {
	if got := vmalertNotifiersURL("https://vmalert.internal.rdcnet.org"); got != "https://vmalert.internal.rdcnet.org/api/v1/notifiers" {
		t.Fatalf("unexpected URL: %s", got)
	}
	if got := vmalertNotifiersURL("https://vmalert.internal.rdcnet.org/api/v1/notifiers"); got != "https://vmalert.internal.rdcnet.org/api/v1/notifiers" {
		t.Fatalf("unexpected URL: %s", got)
	}
}
