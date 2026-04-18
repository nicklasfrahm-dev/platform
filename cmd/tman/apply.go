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

func newApplyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apply <cluster>",
		Short: "Apply Talos configs to all cluster nodes",
		Args:  cobra.ExactArgs(1),
		RunE:  runApply,
	}
	cmd.Flags().Bool("reboot", false, "reboot nodes after applying config")
	return cmd
}

func runApply(cmd *cobra.Command, args []string) error {
	clusterDir := filepath.Join(clusterHome, args[0])
	reboot, _ := cmd.Flags().GetBool("reboot")

	if err := checkDeps("talosctl", "kubectl"); err != nil {
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

	fmt.Printf("Applying Talos configs for cluster: %s\n", meta.Name)
	fmt.Println("==================================================")

	total, failed := 0, 0
	for _, pool := range meta.Pools {
		configFile := filepath.Join(genDir, pool.Name+".yaml")
		if _, err := os.Stat(configFile); err != nil {
			fmt.Fprintf(os.Stderr, "  [pool/%s] ERROR: missing gen/%s.yaml — run 'tman gen' first\n", pool.Name, pool.Name)
			failed += len(pool.Nodes)
			total += len(pool.Nodes)
			continue
		}
		for _, node := range pool.Nodes {
			total++
			if err := applyNode(talosconfig, node, configFile, reboot); err != nil {
				fmt.Fprintf(os.Stderr, "  [%s/%s] ERROR: %v\n", node.Host, node.Name, err)
				failed++
			}
		}
	}

	fmt.Println("==================================================")
	fmt.Printf("Total:   %d\n", total)
	fmt.Printf("Failed:  %d\n", failed)
	fmt.Printf("Success: %d\n", total-failed)

	if failed > 0 {
		return fmt.Errorf("%d node(s) failed", failed)
	}
	return nil
}

func applyNode(talosconfig string, node Node, configFile string, reboot bool) error {
	fmt.Printf("  Applying [%s/%s]\n", node.Host, node.Name)

	config, err := buildNodeConfig(configFile, node)
	if err != nil {
		return fmt.Errorf("build config: %w", err)
	}

	tmp, err := os.CreateTemp("", "tman-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(config); err != nil {
		return err
	}
	tmp.Close()

	base := []string{"--talosconfig", talosconfig, "--endpoints", node.Host, "--nodes", node.Host}
	applyArgs := append(append([]string{}, base...), "apply", "--file", tmp.Name())

	if err := execCmdTimeout(10*time.Second, "talosctl", applyArgs...); err != nil {
		fmt.Printf("  [%s/%s] Normal apply failed, trying maintenance mode\n", node.Host, node.Name)
		insecureArgs := append(applyArgs, "--insecure")
		if err2 := execCmdTimeout(10*time.Second, "talosctl", insecureArgs...); err2 != nil {
			return fmt.Errorf("apply failed (normal and maintenance): %w", err)
		}
		fmt.Printf("  [%s/%s] Applied in maintenance mode\n", node.Host, node.Name)
	} else {
		fmt.Printf("  [%s/%s] Applied successfully\n", node.Host, node.Name)
	}

	if reboot {
		fmt.Printf("  [%s/%s] Rebooting\n", node.Host, node.Name)
		if err := execCmd("talosctl", append(base, "reboot")...); err != nil {
			return fmt.Errorf("reboot: %w", err)
		}
		if err := waitNodeReady(talosconfig, node, 5*time.Minute); err != nil {
			return err
		}
	}
	return nil
}

func buildNodeConfig(configFile string, node Node) ([]byte, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, err
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
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t == "" || isDocKind([]byte(t), excludeKind) {
			continue
		}
		kept = append(kept, t)
	}
	if len(kept) == 0 {
		return nil
	}
	return []byte(strings.Join(kept, "\n---\n") + "\n")
}

func isDocKind(doc []byte, kind string) bool {
	var m struct {
		Kind string `yaml:"kind"`
	}
	_ = yaml.Unmarshal(doc, &m)
	return m.Kind == kind
}

func resolveTalosconfig(clusterDir, genDir string) (string, error) {
	candidates := []string{
		filepath.Join(clusterDir, "talosconfig"),
		filepath.Join(genDir, "talosconfig"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("talosconfig not found in gen/ or cluster root — run 'tman gen' first")
}

func waitNodeReady(talosconfig string, node Node, timeout time.Duration) error {
	fmt.Printf("  [%s/%s] Waiting for node to be ready\n", node.Host, node.Name)
	deadline := time.Now().Add(timeout)
	interval := 5 * time.Second

	for time.Now().Before(deadline) {
		err := execCmdTimeout(5*time.Second, "talosctl",
			"--talosconfig", talosconfig,
			"--nodes", node.Host, "--endpoints", node.Host,
			"version")
		if err == nil {
			break
		}
		time.Sleep(interval)
	}
	if time.Now().After(deadline) {
		return fmt.Errorf("timed out waiting for talos on %s", node.Host)
	}

	if node.Name != "" {
		for time.Now().Before(deadline) {
			err := execCmdTimeout(interval, "kubectl",
				"wait", "node", node.Name,
				"--for=condition=Ready",
				fmt.Sprintf("--timeout=%ds", int(interval.Seconds())))
			if err == nil {
				fmt.Printf("  [%s/%s] Node is ready\n", node.Host, node.Name)
				return nil
			}
		}
		return fmt.Errorf("timed out waiting for kubernetes node %s", node.Name)
	}

	return nil
}
