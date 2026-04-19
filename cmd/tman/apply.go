package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const (
	talosApplyTimeout   = 10 * time.Second
	nodeReadyTimeout    = 5 * time.Minute
	nodePollingInterval = 5 * time.Second
)

func (a *application) newApplyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apply <cluster>",
		Short: "Apply Talos configs to all cluster nodes",
		Args:  cobra.ExactArgs(1),
		RunE:  a.runApply,
	}

	cmd.Flags().Bool("reboot", false, "reboot nodes after applying config")

	return cmd
}

func (a *application) runApply(cmd *cobra.Command, args []string) error {
	clusterDir := filepath.Join(a.clusterHome, args[0])
	reboot, _ := cmd.Flags().GetBool("reboot")

	err := checkDeps("talosctl", "kubectl")
	if err != nil {
		return err
	}

	meta, err := loadMeta(clusterDir)
	if err != nil {
		return err
	}

	genDir := filepath.Join(clusterDir, "gen")

	talosconfig, err := resolveTalosconfig(clusterDir, genDir)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(os.Stdout, "Applying Talos configs for cluster: %s\n", meta.Name)
	_, _ = fmt.Fprintln(os.Stdout, "==================================================")

	total, failed := 0, 0

	for _, pool := range meta.Pools {
		configFile := filepath.Join(genDir, pool.Name+".yaml")

		_, err = os.Stat(configFile)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr,
				"  [pool/%s] ERROR: missing gen/%s.yaml — run 'tman gen' first\n",
				pool.Name, pool.Name)
			failed += len(pool.Nodes)
			total += len(pool.Nodes)

			continue
		}

		for _, node := range pool.Nodes {
			total++

			err = applyNode(talosconfig, node, configFile, reboot)
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "  [%s/%s] ERROR: %v\n", node.Host, node.Name, err)
				failed++
			}
		}
	}

	_, _ = fmt.Fprintln(os.Stdout, "==================================================")
	_, _ = fmt.Fprintf(os.Stdout, "Total:   %d\n", total)
	_, _ = fmt.Fprintf(os.Stdout, "Failed:  %d\n", failed)
	_, _ = fmt.Fprintf(os.Stdout, "Success: %d\n", total-failed)

	if failed > 0 {
		return fmt.Errorf("%d %w", failed, errNodesFailed)
	}

	return nil
}

func applyNode(talosconfig string, node Node, configFile string, reboot bool) error {
	_, _ = fmt.Fprintf(os.Stdout, "  Applying [%s/%s]\n", node.Host, node.Name)

	config, err := buildNodeConfig(configFile, node)
	if err != nil {
		return fmt.Errorf("build config: %w", err)
	}

	tmp, err := os.CreateTemp("", "tman-*.yaml")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	defer func() { _ = os.Remove(tmp.Name()) }()

	_, err = tmp.Write(config)
	if err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	err = tmp.Close()
	if err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	base := []string{"--talosconfig", talosconfig, "--endpoints", node.Host, "--nodes", node.Host}
	applyArgs := append(append([]string{}, base...), "apply", "--file", tmp.Name())

	err = execCmdTimeout(talosApplyTimeout, "talosctl", applyArgs...)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stdout, "  [%s/%s] Normal apply failed, trying maintenance mode\n", node.Host, node.Name)

		insecureArgs := append(append([]string(nil), applyArgs...), "--insecure")

		err2 := execCmdTimeout(talosApplyTimeout, "talosctl", insecureArgs...)
		if err2 != nil {
			return fmt.Errorf("apply failed (normal and maintenance): %w", err)
		}

		_, _ = fmt.Fprintf(os.Stdout, "  [%s/%s] Applied in maintenance mode\n", node.Host, node.Name)
	} else {
		_, _ = fmt.Fprintf(os.Stdout, "  [%s/%s] Applied successfully\n", node.Host, node.Name)
	}

	if reboot {
		_, _ = fmt.Fprintf(os.Stdout, "  [%s/%s] Rebooting\n", node.Host, node.Name)

		err = execTalosctl(append(base, "reboot")...)
		if err != nil {
			return fmt.Errorf("reboot: %w", err)
		}

		err = waitNodeReady(talosconfig, node, nodeReadyTimeout)
		if err != nil {
			return err
		}
	}

	return nil
}

func buildNodeConfig(configFile string, node Node) ([]byte, error) {
	data, err := os.ReadFile(configFile) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var buf bytes.Buffer

	buf.Write(filterDocs(data, "HostnameConfig"))

	if node.Name != "" {
		fmt.Fprintf(&buf, "\n---\napiVersion: v1alpha1\nkind: HostnameConfig\nhostname: %s\n", node.Name)
	}

	return buf.Bytes(), nil
}

var docSepRe = regexp.MustCompile(`(?m)^---\s*$`)

func filterDocs(data []byte, excludeKind string) []byte {
	parts := docSepRe.Split(string(data), -1)

	var kept []string

	for _, doc := range parts {
		trimmed := strings.TrimSpace(doc)
		if trimmed == "" || isDocKind([]byte(trimmed), excludeKind) {
			continue
		}

		kept = append(kept, trimmed)
	}

	if len(kept) == 0 {
		return nil
	}

	return []byte(strings.Join(kept, "\n---\n") + "\n")
}

func isDocKind(doc []byte, kind string) bool {
	var docHeader struct {
		Kind string `yaml:"kind"`
	}

	_ = yaml.Unmarshal(doc, &docHeader)

	return docHeader.Kind == kind
}

func resolveTalosconfig(clusterDir, genDir string) (string, error) {
	candidates := []string{
		filepath.Join(clusterDir, "talosconfig"),
		filepath.Join(genDir, "talosconfig"),
	}

	for _, c := range candidates {
		_, err := os.Stat(c)
		if err == nil {
			return c, nil
		}
	}

	return "", errTalosconfig
}

func waitNodeReady(talosconfig string, node Node, timeout time.Duration) error {
	_, _ = fmt.Fprintf(os.Stdout, "  [%s/%s] Waiting for node to be ready\n", node.Host, node.Name)

	deadline := time.Now().Add(timeout)
	interval := nodePollingInterval

	for time.Now().Before(deadline) {
		err := execCmdTimeout(interval, "talosctl",
			"--talosconfig", talosconfig,
			"--nodes", node.Host, "--endpoints", node.Host,
			"version")
		if err == nil {
			break
		}

		time.Sleep(interval)
	}

	if time.Now().After(deadline) {
		return fmt.Errorf("%w: %s", errTalosTimeout, node.Host)
	}

	if node.Name != "" {
		for time.Now().Before(deadline) {
			err := execCmdTimeout(interval, "kubectl",
				"wait", "node", node.Name,
				"--for=condition=Ready",
				fmt.Sprintf("--timeout=%ds", int(interval.Seconds())))
			if err == nil {
				_, _ = fmt.Fprintf(os.Stdout, "  [%s/%s] Node is ready\n", node.Host, node.Name)

				return nil
			}
		}

		return fmt.Errorf("%w: %s", errKubeTimeout, node.Name)
	}

	return nil
}
