// Package cli wires the cobra subcommands together.
//
// Shared flags (database URL, toxiproxy URL, OTLP endpoint) live on the
// root command so every subcommand inherits them.
package cli

import (
	"time"

	"github.com/spf13/cobra"
)

// CommonFlags holds the flags that every subcommand needs to talk to the
// surrounding infrastructure (postgres, toxiproxy, the OTel collector).
// Subcommands take it by value at parse time.
type CommonFlags struct {
	// DSN is the postgres connection string. By default we connect via
	// toxiproxy so latency injection actually shapes the traffic; bypass
	// it with PGURL=... for a no-toxic baseline.
	DSN string

	// ToxiproxyURL is the toxiproxy HTTP API endpoint (control plane).
	ToxiproxyURL string

	// ToxiproxyProxyName is the named proxy in toxiproxy that fronts
	// postgres. We attach toxics to this proxy.
	ToxiproxyProxyName string

	// OTLPEndpoint is the OTel collector's OTLP/gRPC endpoint for
	// client-side spans. Empty disables client-side tracing.
	OTLPEndpoint string

	// ConnectTimeout caps the initial connection attempt.
	ConnectTimeout time.Duration
}

func bindCommonFlags(cmd *cobra.Command, f *CommonFlags) {
	cmd.PersistentFlags().StringVar(&f.DSN, "dsn",
		"postgres://postgres@localhost:5433/postgres?sslmode=disable",
		"postgres connection string (default points through toxiproxy)")
	cmd.PersistentFlags().StringVar(&f.ToxiproxyURL, "toxiproxy-url",
		"http://localhost:8474",
		"toxiproxy control-plane HTTP API endpoint")
	cmd.PersistentFlags().StringVar(&f.ToxiproxyProxyName, "toxiproxy-proxy",
		"postgres",
		"named toxiproxy proxy that fronts postgres")
	cmd.PersistentFlags().StringVar(&f.OTLPEndpoint, "otlp-endpoint",
		"localhost:4317",
		"OTLP/gRPC endpoint for client-side spans (empty disables)")
	cmd.PersistentFlags().DurationVar(&f.ConnectTimeout, "connect-timeout",
		5*time.Second,
		"initial postgres connection timeout")
}
