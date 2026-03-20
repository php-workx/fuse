package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/runger/fuse/internal/adapters"
)

var proxyCmd = &cobra.Command{
	Use:     "proxy",
	Short:   "Proxy commands (MCP, Codex shell)",
	GroupID: groupRuntime,
}

var proxyDownstreamName string

var proxyMCPCmd = &cobra.Command{
	Use:   "mcp",
	Short: "MCP stdio proxy",
	RunE: func(cmd *cobra.Command, args []string) error {
		if proxyDownstreamName == "" {
			return fmt.Errorf("--downstream-name is required")
		}
		return adapters.RunMCPProxy(proxyDownstreamName, os.Stdin, os.Stdout, os.Stderr)
	},
}

var proxyCodexShellCmd = &cobra.Command{
	Use:   "codex-shell",
	Short: "Codex shell MCP server",
	RunE: func(cmd *cobra.Command, args []string) error {
		return adapters.RunCodexShellServer(os.Stdin, os.Stdout)
	},
}

func init() {
	proxyMCPCmd.Flags().StringVar(&proxyDownstreamName, "downstream-name", "", "Configured downstream MCP proxy name")
	proxyCmd.AddCommand(proxyMCPCmd)
	proxyCmd.AddCommand(proxyCodexShellCmd)
	rootCmd.AddCommand(proxyCmd)
}
