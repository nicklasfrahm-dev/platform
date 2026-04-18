package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func newGenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "gen <cluster>",
		Short: "Generate Talos machine configs for a cluster",
		Args:  cobra.ExactArgs(1),
		RunE:  runGen,
	}
}

func runGen(_ *cobra.Command, args []string) error {
	clusterDir := filepath.Join(clusterHome, args[0])

	if err := checkDeps("talosctl"); err != nil {
		return err
	}

	meta, err := loadMeta(clusterDir)
	if err != nil {
		return err
	}

	patches, err := loadPatches(clusterDir)
	if err != nil {
		return err
	}

	poolPatchPaths := map[string]string{}
	for _, p := range patches {
		if p.Kind == PatchPool {
			poolPatchPaths[p.Pool] = p.Path
		}
	}

	secretBundle := filepath.Join(clusterDir, "secrets.yaml")
	if _, err := os.Stat(secretBundle); os.IsNotExist(err) {
		fmt.Printf("Generating secret bundle: %s\n", secretBundle)
		if err := execCmd("talosctl", "gen", "secrets", "--output-file", secretBundle); err != nil {
			return fmt.Errorf("gen secrets: %w", err)
		}
	}

	tmpDir, err := os.MkdirTemp("", "tman-gen-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	genArgs := []string{
		"gen", "config",
		meta.Name, meta.Endpoint,
		"--with-secrets", secretBundle,
		"--output", tmpDir,
		"--force",
	}
	for _, p := range patches {
		switch p.Kind {
		case PatchBase:
			genArgs = append(genArgs, "--config-patch", "@"+p.Path)
		case PatchCP:
			genArgs = append(genArgs, "--config-patch-control-plane", "@"+p.Path)
		case PatchWorker:
			genArgs = append(genArgs, "--config-patch-worker", "@"+p.Path)
		}
	}
	if err := execCmd("talosctl", genArgs...); err != nil {
		return fmt.Errorf("talosctl gen config: %w", err)
	}

	genDir := filepath.Join(clusterDir, "gen")
	if err := os.RemoveAll(genDir); err != nil {
		return fmt.Errorf("wipe gen dir: %w", err)
	}
	if err := os.MkdirAll(genDir, 0o755); err != nil {
		return err
	}

	if err := copyFile(filepath.Join(tmpDir, "talosconfig"), filepath.Join(clusterDir, "talosconfig")); err != nil {
		return fmt.Errorf("copy talosconfig: %w", err)
	}

	for _, pool := range meta.Pools {
		baseConfig := "controlplane.yaml"
		if pool.Role == "worker" {
			baseConfig = "worker.yaml"
		}
		src := filepath.Join(tmpDir, baseConfig)
		dst := filepath.Join(genDir, pool.Name+".yaml")

		patchFile, hasPatch := poolPatchPaths[pool.Name]
		if hasPatch {
			patchArgs := []string{
				"machineconfig", "patch", src,
				"--patch", "@" + patchFile,
				"--output", dst,
			}
			if err := execCmd("talosctl", patchArgs...); err != nil {
				return fmt.Errorf("patch pool %q: %w", pool.Name, err)
			}
		} else {
			if err := copyFile(src, dst); err != nil {
				return fmt.Errorf("copy config for pool %q: %w", pool.Name, err)
			}
		}

		fmt.Printf("  Generated gen/%s.yaml\n", pool.Name)
	}

	fmt.Printf("Generated configs for cluster: %s\n", meta.Name)
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
