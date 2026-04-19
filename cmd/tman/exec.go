package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

func checkDeps(deps ...string) error {
	for _, dep := range deps {
		_, err := exec.LookPath(dep)
		if err != nil {
			return fmt.Errorf("%w: %s", errMissingDep, dep)
		}
	}

	return nil
}

func execTalosctl(args ...string) error {
	cmd := exec.CommandContext(context.Background(), "talosctl", args...) //nolint:gosec
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("exec: %w", err)
	}

	return nil
}

func execCmdTimeout(timeout time.Duration, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("exec: %w", err)
	}

	return nil
}
