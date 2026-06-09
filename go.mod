module github.com/ringerc/postgres_otel_tracing_bench

go 1.25.0

// Local checkout of github.com/ringerc/pgx_patches, branch
// m-protocol-headers, which adds the pgproto3.RequestHeaders
// message type used by Mode 4. The replace points at a sibling
// worktree; CI / packaging steps should pin the same path or
// remove this directive after the pgx_patches branch lands
// upstream.
replace github.com/jackc/pgx/v5 => ../pgx_patches

require (
	github.com/HdrHistogram/hdrhistogram-go v1.2.0
	github.com/Shopify/toxiproxy/v2 v2.12.0
	github.com/jackc/pgx/v5 v5.10.0
	github.com/spf13/cobra v1.10.2
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	golang.org/x/text v0.29.0 // indirect
)
