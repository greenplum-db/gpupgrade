# abort() is meant to be called from BATS tests. It will exit the process after
# printing its arguments to the TAP stream.
abort() {
    echo "# fatal: $*" 1>&3
    exit 1
}

# require_gpdb() will skip a test if a cluster's environment is not set up.
require_gpdb() {
    [ -n "${GPHOME}" ] || skip "this test requires an active GPDB cluster (set GPHOME)"
    [ -n "${PGPORT}" ] || skip "this test requires an active GPDB cluster (set PGPORT)"
}

# kill_hub() simply kills any gpupgrade_hub process.
# TODO: Killing every running hub is a bad idea, and we don't have any guarantee
# that the signal will have been received by the time we search the ps output.
# Implement a PID file, and use that to kill the hub (and wait for it to exit)
# instead.
kill_hub() {
    pkill -9 gpupgrade_hub || true
    if ps -ef | grep -Gqw "[g]pupgrade_hub"; then
        # Single retry; see TODO above.
        sleep 1
        if ps -ef | grep -Gqw "[g]pupgrade_hub"; then
            abort "didn't kill running hub"
        fi
    fi
}

kill_agents() {
    pkill -9 gpupgrade_agent || true
    if ps -ef | grep -Gqw "[g]pupgrade_agent"; then
        # Single retry; see TODO above.
        sleep 1
        if ps -ef | grep -Gqw "[g]pupgrade_agent"; then
            echo "didn't kill running agents"
        fi
    fi
}
