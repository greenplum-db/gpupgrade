# abort() is meant to be called from BATS tests. It will exit the process after
# printing its arguments to the TAP stream.
abort() {
    echo "# fatal: $@" 1>&3
    exit 1
}

# kill_hub() simply kills any gpupgrade_hub process.
# TODO: Killing every running hub is a bad idea. Implement a PID file and use
# that to kill the hub instead.
kill_hub() {
    pkill -9 gpupgrade_hub || true
    if ps -ef | grep -Gq "[g]pupgrade_hub"; then
        abort "didn't kill running hub"
    fi
}
