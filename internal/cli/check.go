package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewCheckCommand returns a subcommand that verifies the surrounding
// environment is wired up correctly:
//
//   - postgres is reachable via the DSN (and via toxiproxy too).
//   - contrib/otel is loaded (probe by SELECTing from its installed
//     introspection function).
//   - toxiproxy is reachable and has a named proxy matching --toxiproxy-proxy.
//   - the OTLP collector responds to a sentinel span emission.
//
// Use this before a bench run to fail fast on misconfiguration.
func NewCheckCommand() *cobra.Command {
	f := &CommonFlags{}
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Verify postgres / toxiproxy / OTLP collector connectivity",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return fmt.Errorf("TODO: check not yet implemented (dsn=%s, toxiproxy=%s)", f.DSN, f.ToxiproxyURL)
		},
	}
	bindCommonFlags(cmd, f)
	return cmd
}
