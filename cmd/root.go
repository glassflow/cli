package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var (
	verbose    bool
	configPath string
	version    string
	commit     string
	date       string
)

var rootCmd = &cobra.Command{
	Use:   "glassflow",
	Short: "GlassFlow CLI - Local development environment",
	Long:  `GlassFlow CLI provides a local development environment with Kind cluster and demo data.`,
}

func Execute(v, c, d string) {
	version = v
	commit = c
	date = d
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "verbose output")
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "", "path to config file (YAML)")
}
