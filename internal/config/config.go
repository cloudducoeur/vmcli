package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type File struct {
	RefreshInterval time.Duration      `yaml:"refreshInterval"`
	Clusters        map[string]Cluster `yaml:"clusters"`
}

type Cluster struct {
	Vminsert  []string `yaml:"vminsert"`
	Vmselect  []string `yaml:"vmselect"`
	Vmstorage []string `yaml:"vmstorage"`
	Vmalert   []string `yaml:"vmalert"`
	Tenant    Tenant   `yaml:"tenant"`
	Auth      Auth     `yaml:"auth"`
	TLS       TLS      `yaml:"tls"`
}

type Tenant struct {
	AccountID int `yaml:"accountID"`
	ProjectID int `yaml:"projectID"`
}

type Auth struct {
	Username    string `yaml:"username"`
	Password    string `yaml:"password"`
	BearerToken string `yaml:"bearerToken"`
}

type TLS struct {
	CAFile             string `yaml:"caFile"`
	InsecureSkipVerify bool   `yaml:"insecureSkipVerify"`
}

func Load(path string) (File, error) {
	if path == "" {
		path = os.Getenv("VMCLI_CONFIG")
	}
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return File{}, err
		}
		path = filepath.Join(home, ".config", "vmcli", "config.yaml")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return File{}, fmt.Errorf("read config %s: %w", path, err)
	}
	var file File
	if err := yaml.Unmarshal(data, &file); err != nil {
		return File{}, fmt.Errorf("parse config: %w", err)
	}
	if file.RefreshInterval == 0 {
		file.RefreshInterval = 5 * time.Second
	}
	if len(file.Clusters) == 0 {
		return File{}, errors.New("config contains no clusters")
	}
	return file, nil
}
