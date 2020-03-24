#!/usr/bin/env bats

load helpers

setup() {
  skip_if_no_gpdb

  STATE_DIR=`mktemp -d /tmp/gpupgrade.XXXXXX`
  export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"
}

@test "revert restores the system back to untouched state after initialize" {
  # Given initialize has been run
  gpupgrade initialize \
      --source-bindir="$GPHOME/bin" \
      --target-bindir="$GPHOME/bin" \
      --source-master-port="${PGPORT}"\
      --disk-free-ratio 0 \
      --verbose 3>&-

  # When the administrator runs revert
  gpupgrade revert

  # Then the gpupgrade data directory should no longer exist
  [ ! -d "$GPUPGRADE_HOME" ] || fail "found a directory at $GPUPGRADE_HOME, expected revert to remove $GPUPGRADE_HOME"

  # And the gpupgrade data directory should not exist on the host of an agent
  [ "1" -eq "2" ] || fail "TODO"
}