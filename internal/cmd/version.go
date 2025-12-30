package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/certwatch-app/cw-agent/internal/version"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  `Print the version, git commit, and build date of the CertWatch Agent.`,
	Run: func(cmd *cobra.Command, args []string) {
		info := version.GetInfo()
		fmt.Printf("CertWatch Agent\n")
		fmt.Printf("  Version:    %s\n", info.Version)
		fmt.Printf("  Commit:     %s\n", info.GitCommit)
		fmt.Printf("  Build Date: %s\n", info.BuildDate)
		fmt.Printf("  Go Version: %s\n", info.GoVersion)
		fmt.Printf("  Platform:   %s/%s\n", info.OS, info.Arch)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
