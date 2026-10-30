#!/bin/sh
set -eu
# The image's setup helper exits 1 for one keyspace because its last command is ((i++)).
# This unsharded setup needs only the schema directory; keep initialization idempotent.
mkdir -p /vt/schema/commerce
exec /vt/bin/vttestserver \
  --port=15990 --keyspaces=commerce --num-shards=1 \
  --mysql-bind-host=0.0.0.0 --vtcombo-bind-host=0.0.0.0 \
  --mysql-server-version=8.4.6-Vitess \
  --data-dir=/vt/vtdataroot --persistent-mode \
  --schema-dir=/vt/schema --gateway-initial-tablet-timeout=10s
