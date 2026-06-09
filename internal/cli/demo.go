package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// DemoFlags is the parent flag set for `otelbench demo ...` subcommands.
// Individual demos (sqlcommenter-pool-break, etc.) hang off this command.
type DemoFlags struct {
	Common CommonFlags
}

func NewDemoCommand() *cobra.Command {
	f := &DemoFlags{}
	cmd := &cobra.Command{
		Use:   "demo",
		Short: "Run non-timing pathology demonstrations",
		Long: `Demos illustrate problems that don't reduce to a single latency number.

The first such demo is sqlcommenter-pool-break, which shows how
sqlcommenter's per-query SQL-text mutation defeats pgx's automatic
statement cache and blows up pg_stat_statements. This is a pathology
demonstration, not a benchmark --- there is no "good vs bad" comparison;
the point is to show the symptoms.
`,
	}
	bindCommonFlags(cmd, &f.Common)
	cmd.AddCommand(newPoolBreakCommand(&f.Common))
	return cmd
}

func newPoolBreakCommand(common *CommonFlags) *cobra.Command {
	var iterations int
	cmd := &cobra.Command{
		Use:   "sqlcommenter-pool-break",
		Short: "Demonstrate that sqlcommenter prevents pgx's statement cache from working",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return fmt.Errorf("TODO: pool-break demo not yet implemented (iterations=%d, dsn=%s)", iterations, common.DSN)
		},
	}
	cmd.Flags().IntVar(&iterations, "iterations", 10000,
		"how many sqlcommenter-tagged queries to issue")
	return cmd
}
