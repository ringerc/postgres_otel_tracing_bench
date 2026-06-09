# Dockerfile for the bench's "postgres-otel-bench" image.
#
# Multi-stage:
#   1. pg-builder   --- clones the ringerc/postgres tree at the
#                       postgres-otel-tracing branch, builds postgres
#                       in-tree with contrib/otel + contrib/otel_postgres_tracing
#                       enabled (they're tree contribs), installs to
#                       /opt/pgsql.
#   2. demo-builder --- clones postgres_otel_tracing_demo (Rust cdylib),
#                       builds it against the installed postgres's
#                       pg_config, drops the resulting .so into
#                       /opt/pgsql/lib/postgresql/.
#   3. runtime      --- minimal debian + /opt/pgsql + entrypoint that
#                       initdb's a fresh cluster on first start, loads
#                       the extensions via shared_preload_libraries.
#
# The build is slow (~15--20 min cold; postgres ~5 min, rust ~5 min,
# apt/clone the rest). Cached layers cut this dramatically.

ARG POSTGRES_REPO=https://github.com/ringerc/postgres.git
ARG POSTGRES_BRANCH=postgres-otel-tracing
ARG DEMO_REPO=https://github.com/ringerc/postgres_otel_tracing_demo.git
ARG DEMO_BRANCH=main


# --- Stage 1: postgres + in-tree contribs -----------------------------------
FROM debian:bookworm-slim AS pg-builder

# Pinning specific package versions isn't worth the maintenance churn for
# a published-demo image; the base image upgrade rate is conservative
# enough.
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        meson \
        ninja-build \
        pkg-config \
        bison \
        flex \
        perl \
        python3 \
        python3-dev \
        libicu-dev \
        libreadline-dev \
        zlib1g-dev \
        libssl-dev \
        liblz4-dev \
        libzstd-dev \
        git \
        ca-certificates \
        build-essential \
    && rm -rf /var/lib/apt/lists/*

ARG POSTGRES_REPO
ARG POSTGRES_BRANCH
WORKDIR /src
RUN git clone --depth 1 --branch ${POSTGRES_BRANCH} ${POSTGRES_REPO} postgres

# meson setup + build + install. Disabling tap_tests skips the
# perl-tap dependency we don't need at runtime. -Dcassert=false because
# this is a demo image, not a dev one.
WORKDIR /src/postgres
RUN meson setup builddir --prefix=/opt/pgsql -Dtap_tests=disabled -Dcassert=false \
    && ninja -C builddir -j"$(nproc)" install


# --- Stage 2: Rust demo extension -------------------------------------------
FROM rust:1.86-slim AS demo-builder

RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        libclang-dev \
        pkg-config \
        git \
        ca-certificates \
        build-essential \
        protobuf-compiler \
    && rm -rf /var/lib/apt/lists/*

# Bring the postgres install over so cargo can find pg_config + headers.
COPY --from=pg-builder /opt/pgsql /opt/pgsql
ENV PATH=/opt/pgsql/bin:$PATH
ENV PG_CONFIG=/opt/pgsql/bin/pg_config

ARG DEMO_REPO
ARG DEMO_BRANCH
WORKDIR /src
RUN git clone --depth 1 --branch ${DEMO_BRANCH} ${DEMO_REPO} postgres_otel_tracing_demo
WORKDIR /src/postgres_otel_tracing_demo
# meson installs postgres with a multiarch libdir (e.g.
# /opt/pgsql/lib/x86_64-linux-gnu), which is also its pkglibdir --- the
# dir postgres searches for shared_preload_libraries by bare name. Ask
# pg_config rather than hardcoding the triplet so this stays correct
# across architectures.
RUN cargo build --release \
    && install -m 0755 target/release/libpostgres_otel_tracing_demo.so \
        "$(pg_config --pkglibdir)/postgres_otel_tracing_demo.so"


# --- Stage 3: Runtime --------------------------------------------------------
FROM debian:bookworm-slim

RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        libicu72 \
        libreadline8 \
        zlib1g \
        libssl3 \
        liblz4-1 \
        libzstd1 \
        locales \
        ca-certificates \
        tini \
    && echo "en_US.UTF-8 UTF-8" >> /etc/locale.gen \
    && locale-gen \
    && rm -rf /var/lib/apt/lists/*
ENV LANG=en_US.UTF-8 LC_ALL=en_US.UTF-8

RUN groupadd -r postgres && useradd -r -g postgres -d /var/lib/postgresql -m postgres

# Pre-create the data directory and own it correctly so docker's named-
# volume mount inherits the postgres ownership on first start. Without
# this, docker creates the bind point as root and initdb fails with
# permission denied.
RUN mkdir -p /var/lib/postgresql/data \
    && chown -R postgres:postgres /var/lib/postgresql \
    && chmod 0700 /var/lib/postgresql/data

# demo-builder's /opt/pgsql is pg-builder's tree plus the demo .so
# installed into pkglibdir, so a single copy from demo-builder brings
# over both postgres and the demo extension at their correct paths.
COPY --from=demo-builder /opt/pgsql /opt/pgsql

# Bench-specific postgres config + first-boot SQL.
COPY postgresql.conf /opt/pgsql/share/postgresql/postgresql.conf.bench
COPY pg_hba.conf     /opt/pgsql/share/postgresql/pg_hba.conf.bench
COPY init.sh         /usr/local/bin/init.sh
COPY entrypoint.sh   /usr/local/bin/entrypoint.sh
RUN chmod 0755 /usr/local/bin/init.sh /usr/local/bin/entrypoint.sh

ENV PATH=/opt/pgsql/bin:$PATH
ENV PGDATA=/var/lib/postgresql/data

USER postgres
EXPOSE 5432
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/entrypoint.sh"]
CMD ["postgres", "-D", "/var/lib/postgresql/data"]
