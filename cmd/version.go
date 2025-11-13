package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number",
	Long:  `Print the version number, commit hash, and build date of GlassFlow CLI.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("glassflow version %s\n", version)
		if commit != "unknown" && commit != "" {
			fmt.Printf("commit: %s\n", commit)
		}
		if date != "unknown" && date != "" {
			fmt.Printf("date: %s\n", date)
		}
		os.Exit(0)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
