package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var proxyCmd = &cobra.Command{
	Use:   "proxy",
	Short: "Proxy commands (MCP, Codex shell)",
}

var proxyMCPCmd = &cobra.Command{
	Use:   "mcp",
	Short: "MCP stdio proxy (not yet implemented)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("MCP proxy is not yet implemented in v1")
	},
}

func init() {
	proxyCmd.AddCommand(proxyMCPCmd)
	rootCmd.AddCommand(proxyCmd)
}
