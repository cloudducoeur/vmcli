package models

import "time"

// Component identifies a VictoriaMetrics cluster component.
type Component string

const (
	Vminsert  Component = "vminsert"
	Vmselect  Component = "vmselect"
	Vmstorage Component = "vmstorage"
)

// Node is a configured VictoriaMetrics endpoint and its latest observed state.
type Node struct {
	Component Component
	URL       string
	Status    string
	Version   string
	Uptime    string
	Metrics   map[string]float64
	Error     string
	CheckedAt time.Time
}
