package cluster

import (
	"context"
	"net/url"
	"sync"
	"time"

	"vmcli/internal/api"
	"vmcli/internal/config"
	"vmcli/internal/models"
)

func Nodes(c config.Cluster) []models.Node {
	var nodes []models.Node
	for component, endpoints := range map[models.Component][]string{models.Vminsert: c.Vminsert, models.Vmselect: c.Vmselect, models.Vmstorage: c.Vmstorage} {
		for _, endpoint := range endpoints {
			nodes = append(nodes, models.Node{Component: component, URL: endpoint, Status: "UNKNOWN"})
		}
	}
	return nodes
}

func Check(ctx context.Context, client api.VictoriaMetricsClient, nodes []models.Node) []models.Node {
	result := append([]models.Node(nil), nodes...)
	var wait sync.WaitGroup
	for index := range result {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			metrics, err := client.Metrics(checkCtx, result[index].URL)
			result[index].CheckedAt = time.Now()
			if err != nil {
				result[index].Status = "DOWN"
				result[index].Error = err.Error()
				return
			}
			result[index].Status = "UP"
			result[index].Metrics = metrics
			if parsed, err := url.Parse(result[index].URL); err == nil {
				result[index].URL = parsed.Host
			}
		}(index)
	}
	wait.Wait()
	return result
}
