#!/bin/sh
# Image entrypoint.
#
# 1. Run init.sh to bootstrap the cluster on first boot (idempotent).
# 2. exec into postgres as the foreground process so docker handles
#    signals and logs correctly.

set -e

# init.sh handles its own no-op-on-already-initialised check.
/usr/local/bin/init.sh

exec "$@"
