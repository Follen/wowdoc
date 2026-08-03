package app

import (
	"io"

	"github.com/follenfang/wowdoc/internal/result"
	"github.com/spf13/cobra"
)

var Version = "0.1.0-dev"
var Commit = "unknown"

func RunWowdoc(args []string, stdout, stderr io.Writer) int {
	return execute(newWowdoc(), args, stdout, stderr)
}
func execute(root *cobra.Command, args []string, stdout, stderr io.Writer) int {
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SilenceErrors = true
	root.SilenceUsage = true
	if err := root.Execute(); err != nil {
		return result.WriteError(stdout, stderr, err)
	}
	return 0
}

func require(value, code, message string) error {
	if value == "" {
		return result.E(code, message, 2)
	}
	return nil
}
