package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var clusterHome string

func main() {
	root := &cobra.Command{
		Use:          "tman",
		Short:        "Talos cluster manager",
		SilenceUsage: true,
	}
	root.PersistentFlags().StringVar(&clusterHome, "cluster-home", "clusters", "base directory containing cluster directories")
	root.AddCommand(newGenCmd(), newApplyCmd())
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
