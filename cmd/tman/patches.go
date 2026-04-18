package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type PatchKind int

const (
	PatchBase PatchKind = iota
	PatchCP
	PatchWorker
	PatchPool
)

type Patch struct {
	Path string
	Kind PatchKind
	Pool string
}

var poolRe = regexp.MustCompile(`^20-pool-(.+)\.yaml$`)

func classifyPatch(path string) Patch {
	base := filepath.Base(path)
	switch {
	case strings.HasPrefix(base, "00-"):
		return Patch{Path: path, Kind: PatchBase}
	case strings.HasPrefix(base, "10-role-cp"):
		return Patch{Path: path, Kind: PatchCP}
	case strings.HasPrefix(base, "10-role-worker"):
		return Patch{Path: path, Kind: PatchWorker}
	default:
		if m := poolRe.FindStringSubmatch(base); m != nil {
			return Patch{Path: path, Kind: PatchPool, Pool: m[1]}
		}
		return Patch{Path: path, Kind: PatchBase}
	}
}

func loadPatches(clusterDir string) ([]Patch, error) {
	files, err := filepath.Glob(filepath.Join(clusterDir, "patches", "*.yaml"))
	if err != nil {
		return nil, fmt.Errorf("glob patches: %w", err)
	}
	sort.Strings(files)
	patches := make([]Patch, len(files))
	for i, f := range files {
		patches[i] = classifyPatch(f)
	}
	return patches, nil
}
