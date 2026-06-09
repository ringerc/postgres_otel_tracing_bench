// otelbench --- a benchmark + demo harness for comparing trace-context
// propagation methods against PostgreSQL with contrib/otel.
//
// Subcommands:
//
//	bench   - run the timing benchmark across one or more modes
//	demo    - run the non-timing demonstrations (sqlcommenter pool break)
//	check   - one-shot environment + connectivity checks
//
// See README.md for the mode definitions and the protocol-level rationale.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/ringerc/postgres_otel_tracing_bench/internal/cli"
)

func main() {
	root := &cobra.Command{
		Use:           "otelbench",
		Short:         "PostgreSQL trace-context propagation benchmark and demo harness",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(
		cli.NewBenchCommand(),
		cli.NewDemoCommand(),
		cli.NewCheckCommand(),
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := root.ExecuteContext(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			os.Exit(130)
		}
		fmt.Fprintf(os.Stderr, "otelbench: %v\n", err)
		os.Exit(1)
	}
}
