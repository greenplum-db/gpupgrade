package greenplum

import (
	"fmt"
	"os/exec"
)

type pgrepCommand struct {
	streams OutStreams
}

var pgrepCmd = exec.Command

func newPgrepCommand(stream OutStreams) *pgrepCommand {
	return &pgrepCommand{streams: stream}
}

func (m *pgrepCommand) isRunning(pidFile string) error {
	cmd := pgrepCmd("bash", "-c", fmt.Sprintf("pgrep -F %s", pidFile))

	cmd.Stdout = m.streams.Stdout()
	cmd.Stderr = m.streams.Stderr()

	return cmd.Run()
}
