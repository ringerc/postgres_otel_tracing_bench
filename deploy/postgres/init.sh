#!/bin/sh
# First-boot init script for the bench postgres image.
#
# Runs initdb if PGDATA doesn't exist yet, copies our bench-specific
# postgresql.conf and pg_hba.conf on top of initdb's defaults, then
# starts postgres temporarily to CREATE EXTENSION the three otel
# extensions + pg_stat_statements (which can only be created against
# a running cluster, not via initdb).
#
# Called by entrypoint.sh; idempotent on subsequent boots (does nothing
# if PGDATA is already initialised).

set -e

if [ -s "$PGDATA/PG_VERSION" ]; then
    # Already initialised; nothing to do.
    exit 0
fi

echo "init.sh: bootstrapping fresh cluster in $PGDATA"

# initdb. Trust auth so the bench can connect without a password.
initdb \
    --pgdata="$PGDATA" \
    --username=postgres \
    --auth-host=trust \
    --auth-local=trust \
    --encoding=UTF8 \
    --locale=C.UTF-8 \
    --no-sync

# Overwrite the generated config files with our bench-tuned ones.
install -m 0600 /opt/pgsql/share/postgresql/postgresql.conf.bench "$PGDATA/postgresql.conf"
install -m 0600 /opt/pgsql/share/postgresql/pg_hba.conf.bench     "$PGDATA/pg_hba.conf"

# Start postgres locally on a unix socket only, just long enough to
# CREATE the extensions. Listening on TCP here would expose the
# bootstrap cluster externally during the brief startup window.
echo "init.sh: starting cluster to create extensions"
pg_ctl -D "$PGDATA" -l /tmp/init-pg.log -o "-c listen_addresses='' -k /tmp" -w start

createdb_query() {
    psql -h /tmp -U postgres -d postgres -v ON_ERROR_STOP=1 -c "$1"
}

createdb_query "CREATE EXTENSION IF NOT EXISTS otel_api;"
createdb_query "CREATE EXTENSION IF NOT EXISTS otel_postgres_tracing;"
createdb_query "CREATE EXTENSION IF NOT EXISTS pg_stat_statements;"
# postgres_otel_tracing_demo is a preload-only module (no SQL surface,
# no .control file), so no CREATE EXTENSION for it --- listing it in
# shared_preload_libraries is enough.

echo "init.sh: stopping cluster; entrypoint will restart with TCP enabled"
pg_ctl -D "$PGDATA" -w stop
