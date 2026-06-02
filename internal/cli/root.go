package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func NewRootCommand() *cobra.Command {
	root := &cobra.Command{Use: "wowdoc", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(apiCommand())
	root.AddCommand(clientsCommand())
	root.AddCommand(mcpCommand())
	return root
}

func apiCommand() *cobra.Command {
	api := &cobra.Command{Use: "api"}
	var client, ref, name, sourceRoot, sourcePath string
	lookup := &cobra.Command{
		Use:   "lookup",
		Short: "Lookup a Blizzard API symbol.",
		Long: `Lookup a Blizzard API symbol.

Required:
  --client retail|classic|classic-ptr|classic-titan|ptr2|<discovered alias>
  --name API_NAME

Optional:
  --ref REF       branch, tag, or commit. Defaults to the client's latest ref.

Source resolution:
  --source-path wins over --source-root + --client + --ref.
  If no source root is set, wowdoc uses <exe-dir>/sources.

Agent next step:
  If client is unknown, run: wowdoc clients list --include-diagnostics

Minimum valid call:
  wowdoc api lookup --client retail --name C_AuctionHouse.GetItemSearchResultInfo

MCP arguments:
  {"client":"retail","name":"C_AuctionHouse.GetItemSearchResultInfo"}

Common error codes:
  client_required client_not_found source_not_found source_invalid ref_not_found git_unavailable_archive_failed capability_unavailable index_unavailable timeout unsupported_ref`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if client == "" {
				return errors.New("client_required")
			}
			if name == "" {
				return errors.New("name_required")
			}
			_ = ref
			_ = sourceRoot
			_ = sourcePath
			return nil
		},
	}
	lookup.Flags().StringVar(&client, "client", "", "source client alias")
	lookup.Flags().StringVar(&ref, "ref", "", "branch, tag, or commit")
	lookup.Flags().StringVar(&name, "name", "", "API name")
	lookup.Flags().StringVar(&sourceRoot, "source-root", "", "source cache root")
	lookup.Flags().StringVar(&sourcePath, "source-path", "", "explicit source checkout path")
	api.AddCommand(lookup)
	return api
}

func clientsCommand() *cobra.Command {
	clients := &cobra.Command{Use: "clients"}
	var includeDiagnostics bool
	list := &cobra.Command{Use: "list", RunE: func(cmd *cobra.Command, args []string) error {
		_ = includeDiagnostics
		return nil
	}}
	list.Flags().BoolVar(&includeDiagnostics, "include-diagnostics", false, "include invalid source diagnostics")
	clients.AddCommand(list)
	return clients
}

func mcpCommand() *cobra.Command {
	mcp := &cobra.Command{Use: "mcp"}
	mcp.AddCommand(&cobra.Command{Use: "stdio", RunE: func(cmd *cobra.Command, args []string) error { return nil }})
	return mcp
}
