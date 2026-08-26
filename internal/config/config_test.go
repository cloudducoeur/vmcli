package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte("refreshInterval: 2s\nclusters:\n  prod:\n    vminsert: [http://insert:8480]\n    vmselect: [http://select:8481]\n    vmstorage: [http://storage:8482]\n    vmalert: [http://vmalert:8880]\n")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if file.RefreshInterval != 2*time.Second || len(file.Clusters["prod"].Vmstorage) != 1 || len(file.Clusters["prod"].Vmalert) != 1 {
		t.Fatalf("unexpected config: %+v", file)
	}
}
