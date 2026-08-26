package api

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"vmcli/internal/config"
)

type VictoriaMetricsClient interface {
	Health(context.Context, string) error
	Metrics(context.Context, string) (map[string]float64, error)
	Query(context.Context, string, int, int, string) (QueryResult, error)
	QueryRange(context.Context, string, int, int, string, time.Time, time.Time, time.Duration) (QueryResult, error)
	VmalertRules(context.Context, string) (VmalertRulesResponse, error)
	VmalertAlerts(context.Context, string) (VmalertAlertsResponse, error)
	VmalertNotifiers(context.Context, string) (VmalertNotifiersResponse, error)
}

type QueryResult struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string            `json:"resultType"`
		Result     []json.RawMessage `json:"result"`
	} `json:"data"`
}

type VmalertRulesResponse struct {
	Status string `json:"status"`
	Data   struct {
		Groups []VmalertRuleGroup `json:"groups"`
	} `json:"data"`
}

type VmalertRuleGroup struct {
	Name  string        `json:"name"`
	Rules []VmalertRule `json:"rules"`
}

type VmalertRule struct {
	State             string            `json:"state"`
	Name              string            `json:"name"`
	Query             string            `json:"query"`
	Duration          int               `json:"duration"`
	KeepFiringFor     int               `json:"keep_firing_for"`
	Labels            map[string]string `json:"labels"`
	Annotations       map[string]string `json:"annotations"`
	LastError         string            `json:"lastError"`
	EvaluationTime    float64           `json:"evaluationTime"`
	LastEvaluation    string            `json:"lastEvaluation"`
	Health            string            `json:"health"`
	Type              string            `json:"type"`
	LastSamples       int               `json:"lastSamples"`
	LastSeriesFetched int               `json:"lastSeriesFetched"`
	File              string            `json:"file"`
}

type VmalertAlertsResponse struct {
	Status string `json:"status"`
	Data   struct {
		Alerts []VmalertAlert `json:"alerts"`
	} `json:"data"`
}

type VmalertAlert struct {
	Name        string            `json:"name"`
	GroupName   string            `json:"group_name"`
	GroupID     string            `json:"group_id"`
	State       string            `json:"state"`
	Value       string            `json:"value"`
	ActiveAt    string            `json:"activeAt"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
}

type VmalertNotifiersResponse struct {
	Status string `json:"status"`
	Data   struct {
		Notifiers []VmalertNotifier `json:"notifiers"`
	} `json:"data"`
}

type VmalertNotifier struct {
	Kind    string                  `json:"kind"`
	Targets []VmalertNotifierTarget `json:"targets"`
}

type VmalertNotifierTarget struct {
	Address   string            `json:"address"`
	Labels    map[string]string `json:"labels"`
	LastError string            `json:"lastError"`
}

type Client struct {
	httpClient *http.Client
	auth       config.Auth
}

func NewClient(tlsConfig config.TLS, auth config.Auth) (*Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()

	if tlsConfig.InsecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec: explicit user configuration
	} else if tlsConfig.CAFile != "" {
		data, err := os.ReadFile(tlsConfig.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read CA file: %w", err)
		}
		pool, err := x509.SystemCertPool()
		if err != nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(data) {
			return nil, fmt.Errorf("CA file contains no certificates")
		}
		transport.TLSClientConfig = &tls.Config{RootCAs: pool}
	}
	return &Client{httpClient: &http.Client{Transport: transport}, auth: auth}, nil
}

func (c *Client) request(ctx context.Context, method, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(endpoint, "/"), nil)
	if err != nil {
		return nil, err
	}
	if c.auth.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.auth.BearerToken)
	} else if c.auth.Username != "" {
		req.SetBasicAuth(c.auth.Username, c.auth.Password)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &HTTPError{StatusCode: resp.StatusCode, Body: string(body)}
	}
	return body, nil
}

type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string { return fmt.Sprintf("HTTP %d", e.StatusCode) }

func (c *Client) Health(ctx context.Context, endpoint string) error {
	_, err := c.request(ctx, http.MethodGet, endpoint+"/health")
	return err
}

func (c *Client) Metrics(ctx context.Context, endpoint string) (map[string]float64, error) {
	body, err := c.request(ctx, http.MethodGet, endpoint+"/metrics")
	if err != nil {
		return nil, err
	}
	return ParseMetrics(string(body)), nil
}

func (c *Client) Query(ctx context.Context, endpoint string, accountID, projectID int, query string) (QueryResult, error) {
	queryURL := fmt.Sprintf("%s/select/%d/prometheus/api/v1/query?query=%s", strings.TrimRight(endpoint, "/"), projectID, url.QueryEscape(query))
	body, err := c.request(ctx, http.MethodGet, queryURL)
	if err != nil {
		return QueryResult{}, err
	}
	var result QueryResult
	err = json.Unmarshal(body, &result)
	return result, err
}

func (c *Client) QueryRange(ctx context.Context, endpoint string, accountID, projectID int, query string, start, end time.Time, step time.Duration) (QueryResult, error) {
	values := url.Values{}
	values.Set("query", query)
	values.Set("start", fmt.Sprintf("%d", start.Unix()))
	values.Set("end", fmt.Sprintf("%d", end.Unix()))
	values.Set("step", step.String())
	queryURL := fmt.Sprintf("%s/select/%d/prometheus/api/v1/query_range?%s", strings.TrimRight(endpoint, "/"), projectID, values.Encode())
	body, err := c.request(ctx, http.MethodGet, queryURL)
	if err != nil {
		return QueryResult{}, err
	}
	var result QueryResult
	err = json.Unmarshal(body, &result)
	return result, err
}

func (c *Client) VmalertRules(ctx context.Context, endpoint string) (VmalertRulesResponse, error) {
	body, err := c.request(ctx, http.MethodGet, vmalertRulesURL(endpoint))
	if err != nil {
		return VmalertRulesResponse{}, err
	}
	var result VmalertRulesResponse
	err = json.Unmarshal(body, &result)
	return result, err
}

func (c *Client) VmalertAlerts(ctx context.Context, endpoint string) (VmalertAlertsResponse, error) {
	body, err := c.request(ctx, http.MethodGet, vmalertAlertsURL(endpoint))
	if err != nil {
		return VmalertAlertsResponse{}, err
	}
	var result VmalertAlertsResponse
	err = json.Unmarshal(body, &result)
	return result, err
}

func (c *Client) VmalertNotifiers(ctx context.Context, endpoint string) (VmalertNotifiersResponse, error) {
	body, err := c.request(ctx, http.MethodGet, vmalertNotifiersURL(endpoint))
	if err != nil {
		return VmalertNotifiersResponse{}, err
	}
	var result VmalertNotifiersResponse
	err = json.Unmarshal(body, &result)
	return result, err
}

func vmalertRulesURL(endpoint string) string {
	return vmalertAPIURL(endpoint, "/api/v1/rules")
}

func vmalertAlertsURL(endpoint string) string {
	return vmalertAPIURL(endpoint, "/api/v1/alerts")
}

func vmalertNotifiersURL(endpoint string) string {
	return vmalertAPIURL(endpoint, "/api/v1/notifiers")
}

func vmalertAPIURL(endpoint, suffix string) string {
	trimmed := strings.TrimRight(endpoint, "/")
	if strings.HasSuffix(trimmed, suffix) {
		return trimmed
	}
	return trimmed + suffix
}

func ParseMetrics(text string) map[string]float64 {
	metrics := make(map[string]float64)
	for _, line := range strings.Split(text, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		var value float64
		if _, err := fmt.Sscanf(fields[len(fields)-1], "%f", &value); err == nil {
			metrics[strings.Split(fields[0], "{")[0]] = value
		}
	}
	return metrics
}
