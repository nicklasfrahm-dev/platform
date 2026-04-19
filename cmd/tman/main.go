// Package main provides the tman CLI for managing Talos clusters.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

type application struct {
	clusterHome string
}

func main() {
	app := &application{}

	root := &cobra.Command{
		Use:          "tman",
		Short:        "Talos cluster manager",
		SilenceUsage: true,
	}

	root.PersistentFlags().StringVar(
		&app.clusterHome,
		"cluster-home",
		"clusters",
		"base directory containing cluster directories",
	)

	root.AddCommand(app.newGenCmd(), app.newApplyCmd())

	err := root.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
