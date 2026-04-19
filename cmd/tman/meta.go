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
	data, err := os.ReadFile(filepath.Join(clusterDir, "meta.yaml")) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("read meta.yaml: %w", err)
	}

	var meta Meta

	err = yaml.Unmarshal(data, &meta)
	if err != nil {
		return nil, fmt.Errorf("parse meta.yaml: %w", err)
	}

	err = validateMeta(&meta)
	if err != nil {
		return nil, err
	}

	return &meta, nil
}

func validateMeta(meta *Meta) error {
	if meta.Name == "" {
		return errMissingName
	}

	if meta.Endpoint == "" {
		return errMissingEndpoint
	}

	if len(meta.Pools) == 0 {
		return errNoPools
	}

	for _, pool := range meta.Pools {
		err := validatePool(pool)
		if err != nil {
			return err
		}
	}

	return nil
}

func validatePool(pool Pool) error {
	if pool.Name == "" {
		return errPoolMissingName
	}

	if pool.Role != "controlplane" && pool.Role != "worker" {
		return fmt.Errorf("meta.yaml: pool %q: %w", pool.Name, errInvalidRole)
	}

	for _, node := range pool.Nodes {
		if node.Host == "" {
			return fmt.Errorf("meta.yaml: pool %q: node %q: %w", pool.Name, node.Name, errMissingHost)
		}
	}

	return nil
}
