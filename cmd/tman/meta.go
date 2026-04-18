package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Node struct {
	Name string `yaml:"name"`
	Host string `yaml:"host"`
}

type Pool struct {
	Name  string `yaml:"name"`
	Role  string `yaml:"role"` // "controlplane" or "worker"
	Nodes []Node `yaml:"nodes"`
}

type Meta struct {
	Name     string `yaml:"name"`
	Endpoint string `yaml:"endpoint"`
	Pools    []Pool `yaml:"pools"`
}

func loadMeta(clusterDir string) (*Meta, error) {
	data, err := os.ReadFile(filepath.Join(clusterDir, "meta.yaml"))
	if err != nil {
		return nil, fmt.Errorf("read meta.yaml: %w", err)
	}
	var m Meta
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse meta.yaml: %w", err)
	}
	if m.Name == "" {
		return nil, fmt.Errorf("meta.yaml: missing name")
	}
	if m.Endpoint == "" {
		return nil, fmt.Errorf("meta.yaml: missing endpoint")
	}
	if len(m.Pools) == 0 {
		return nil, fmt.Errorf("meta.yaml: no pools defined")
	}
	for _, p := range m.Pools {
		if p.Name == "" {
			return nil, fmt.Errorf("meta.yaml: pool missing name")
		}
		if p.Role != "controlplane" && p.Role != "worker" {
			return nil, fmt.Errorf("meta.yaml: pool %q: role must be 'controlplane' or 'worker'", p.Name)
		}
		for _, n := range p.Nodes {
			if n.Host == "" {
				return nil, fmt.Errorf("meta.yaml: pool %q: node %q missing host", p.Name, n.Name)
			}
		}
	}
	return &m, nil
}
