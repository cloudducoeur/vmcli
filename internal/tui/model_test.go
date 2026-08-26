package tui

import (
	"testing"
	"time"

	"vmcli/internal/config"
)

func TestNewModel(t *testing.T) {
	model := New(nil, config.Cluster{}, time.Second)
	if model.interval != time.Second {
		t.Fatal(model.interval)
	}
}
