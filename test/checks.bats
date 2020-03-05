#! /usr/bin/env bats

load helpers

setup() {
    skip_if_no_gpdb

    STATE_DIR=`mktemp -d`
    export GPUPGRADE_HOME="${STATE_DIR}/gpupgrade"

    gpupgrade kill-services
}

teardown() {
    skip_if_no_gpdb

    gpupgrade kill-services
    rm -r "$STATE_DIR"
}

# Writes the available disk space, in KiB, on the filesystem containing the
# given path. The STAT global must be set (see require_gnu_stat).
avail_disk_space() {
    local path=$1

    local bsize bavail
    read -r bsize bavail <<< $($STAT -f -c '%S %a' "$path")

    echo $(( $bavail * $bsize / 1024 ))
}

# Writes the total disk space in KiB, not including reserved superuser blocks,
# on the filesystem containing the given path. The STAT global must be set (see
# require_gnu_stat).
total_disk_space() {
    local path=$1

    local bavail bsize bfree btotal
    read -r bsize bavail bfree btotal <<< $($STAT -f -c '%S %a %f %b' "$path")

    echo $(( ( $btotal - $bfree + $bavail ) * $bsize / 1024 ))
}

# Converts pretty-printed byte values to raw KiB for numerical comparison.
convert_to_kib() {
    local value=$1
    local unit=$2

    # Map byte prefixes to factors of 1024 via string indexes.
    # e.g. MiB -> 1024^1 KiB, PiB -> 1024^4 KiB, etc.
    local indexstr="KMGTPE"
    local index=${indexstr%%$unit*}
    local exponent=${#index}

    # We use awk here because bc isn't available in all test environments and
    # Bash doesn't do floating-point math natively.
    awk "BEGIN { printf \"%d\", $value * 1024^$exponent }"
}

# Verify two numbers are within a specified percent tolerance.
are_equivalent_within_tolerance() {
    local actual=$1
    local expected=$2
    local tolerance=$3

    local delta=$(( actual - expected ))
    if [ "$delta" -lt 0 ]; then
        (( delta = -delta ))
    fi

    local max_delta=$(awk "BEGIN { printf \"%d\", $expected * $tolerance }")
    [ "$delta" -le "$max_delta" ]
}

@test "initialize prints disk space on failure" {
    require_gnu_stat

    datadir=$(psql postgres -Atc "select datadir from gp_segment_configuration where role='p' and content=-1")

    run gpupgrade initialize \
        --disk-free-ratio=1.0 \
        --source-bindir="$PWD" \
        --target-bindir="$PWD" \
        --source-master-port="${PGPORT}" \
        --stop-before-cluster-creation 3>&-

    [ "$status" -eq 1 ]

    # XXX Currently, we assume a single-host demo cluster.
    pattern='You currently do not have enough disk space to run an upgrade\..+'
    pattern+='Hostname +Filesystem +Shortfall +Available +Required.+'
    pattern+="$(hostname)"' +/[^ ]* +[.[:digit:]]+ [KMGTPE]iB +([.[:digit:]]+) ([KMGTPE])iB +([.[:digit:]]+) ([KMGTPE])iB'

    [[ $output =~ $pattern ]] || fail "actual output: $output"

    available="${BASH_REMATCH[1]}"
    available_unit="${BASH_REMATCH[2]}"
    available_bytes=$(convert_to_kib $available $available_unit)
    required="${BASH_REMATCH[3]}"
    required_unit="${BASH_REMATCH[4]}"
    required_bytes=$(convert_to_kib $required $required_unit)

    # Validating the shortfall is hard due to rounding errors from our
    # pretty-print code. Defer to unit tests for that and just check avail and
    # total.

    avail_space=$(avail_disk_space "$datadir")
    if ! are_equivalent_within_tolerance $available_bytes $avail_space 0.001; then
        fail "the available bytes ($available_bytes) are not within 0.1% of the avail disk space ($avail_space)"
    fi

    total_space=$(total_disk_space "$datadir")
    if ! are_equivalent_within_tolerance $required_bytes $total_space 0.001; then
        fail "the required bytes ($required_bytes) are not within 0.1% of the total disk space ($total_space)"
    fi
}
