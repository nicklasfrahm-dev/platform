package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

const genDirPerm = 0o750

func (a *application) newGenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "gen <cluster>",
		Short: "Generate Talos machine configs for a cluster",
		Args:  cobra.ExactArgs(1),
		RunE:  a.runGen,
	}
}

func (a *application) runGen(_ *cobra.Command, args []string) error {
	clusterDir := filepath.Join(a.clusterHome, args[0])

	err := checkDeps("talosctl")
	if err != nil {
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

	secretBundle, err := ensureSecretBundle(clusterDir)
	if err != nil {
		return err
	}

	tmpDir, cleanup, err := runTalosGenConfig(meta, patches, secretBundle)
	if err != nil {
		return err
	}

	defer cleanup()

	genDir := filepath.Join(clusterDir, "gen")

	err = installGeneratedConfigs(clusterDir, genDir, tmpDir, meta, patches)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(os.Stdout, "Generated configs for cluster: %s\n", meta.Name)

	return nil
}

func ensureSecretBundle(clusterDir string) (string, error) {
	secretBundle := filepath.Join(clusterDir, "secrets.yaml")

	_, err := os.Stat(secretBundle)
	if os.IsNotExist(err) {
		_, _ = fmt.Fprintf(os.Stdout, "Generating secret bundle: %s\n", secretBundle)

		err = execTalosctl("gen", "secrets", "--output-file", secretBundle)
		if err != nil {
			return "", fmt.Errorf("gen secrets: %w", err)
		}
	}

	return secretBundle, nil
}

func runTalosGenConfig(meta *Meta, patches []Patch, secretBundle string) (string, func(), error) {
	tmpDir, err := os.MkdirTemp("", "tman-gen-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp dir: %w", err)
	}

	cleanup := func() { _ = os.RemoveAll(tmpDir) }

	genArgs := buildGenArgs(meta, secretBundle, tmpDir, patches)

	err = execTalosctl(genArgs...)
	if err != nil {
		cleanup()

		return "", nil, fmt.Errorf("talosctl gen config: %w", err)
	}

	return tmpDir, cleanup, nil
}

func buildGenArgs(meta *Meta, secretBundle, outputDir string, patches []Patch) []string {
	args := []string{
		"gen", "config",
		meta.Name, meta.Endpoint,
		"--with-secrets", secretBundle,
		"--output", outputDir,
		"--force",
	}

	for _, patch := range patches {
		switch patch.Kind {
		case PatchBase:
			args = append(args, "--config-patch", "@"+patch.Path)
		case PatchCP:
			args = append(args, "--config-patch-control-plane", "@"+patch.Path)
		case PatchWorker:
			args = append(args, "--config-patch-worker", "@"+patch.Path)
		case PatchPool:
			// applied per-pool in installGeneratedConfigs
		}
	}

	return args
}

func installGeneratedConfigs(clusterDir, genDir, tmpDir string, meta *Meta, patches []Patch) error {
	poolPatchPaths := buildPoolPatchPaths(patches)

	err := os.RemoveAll(genDir)
	if err != nil {
		return fmt.Errorf("wipe gen dir: %w", err)
	}

	err = os.MkdirAll(genDir, genDirPerm)
	if err != nil {
		return fmt.Errorf("create gen dir: %w", err)
	}

	err = copyFile(filepath.Join(tmpDir, "talosconfig"), filepath.Join(clusterDir, "talosconfig"))
	if err != nil {
		return fmt.Errorf("copy talosconfig: %w", err)
	}

	for _, pool := range meta.Pools {
		err = generatePoolConfig(pool, tmpDir, genDir, poolPatchPaths)
		if err != nil {
			return err
		}
	}

	return nil
}

func buildPoolPatchPaths(patches []Patch) map[string]string {
	paths := map[string]string{}

	for _, patch := range patches {
		if patch.Kind == PatchPool {
			paths[patch.Pool] = patch.Path
		}
	}

	return paths
}

func generatePoolConfig(pool Pool, tmpDir, genDir string, poolPatchPaths map[string]string) error {
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

		err := execTalosctl(patchArgs...)
		if err != nil {
			return fmt.Errorf("patch pool %q: %w", pool.Name, err)
		}
	} else {
		err := copyFile(src, dst)
		if err != nil {
			return fmt.Errorf("copy config for pool %q: %w", pool.Name, err)
		}
	}

	_, _ = fmt.Fprintf(os.Stdout, "  Generated gen/%s.yaml\n", pool.Name)

	return nil
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src) //nolint:gosec
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}

	defer func() { _ = srcFile.Close() }()

	out, err := os.Create(dst) //nolint:gosec
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}

	_, err = io.Copy(out, srcFile)

	closeErr := out.Close()
	if closeErr != nil && err == nil {
		err = closeErr
	}

	if err != nil {
		return fmt.Errorf("copy %s: %w", src, err)
	}

	return nil
}
