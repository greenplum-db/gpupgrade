package services

import (
	"context"
	"os"
	"path/filepath"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	pb "github.com/greenplum-db/gpupgrade/idl"
)

// grpc generated function signature requires ctx and in params.
// nolint: unparam
func (h *Hub) CheckSeginstall(ctx context.Context, in *pb.CheckSeginstallRequest) (*pb.CheckSeginstallReply, error) {
	gplog.Info("starting CheckSeginstall()")
	hostnames := h.clusterPair.GetHostnames()
	agentPath := filepath.Join(os.Getenv("GPHOME"), "bin", "gpupgrade_agent")
	command := []string{"ls", agentPath}
	go func() {
		var anyFailed = false
		err := h.checklistWriter.ResetStateDir(upgradestatus.SEGINSTALL)
		if err != nil {
			gplog.Error(err.Error())
			//For MMVP, return here, but maybe should log more info
			goto fail
		}
		err = h.checklistWriter.MarkInProgress(upgradestatus.SEGINSTALL)
		if err != nil {
			gplog.Error(err.Error())
			//For MMVP, return here, but maybe should log more info
			goto fail
		}

		//default assumption: GPDB is installed on the same path on all hosts in cluster
		//we're looking for gpupgrade_agent as proof that the new binary is installed
		//TODO: if this finds nothing, should we err out? do a fallback check based on $GPHOME?
		for _, hostname := range hostnames {
			sshArgs := []string{"-o", "StrictHostKeyChecking=no", hostname}
			sshArgs = append(sshArgs, command...)
			output, err := h.commandExecer("ssh", sshArgs...).CombinedOutput()
			//TODO: fix the string formatting. don't include untrusted output in the format string
			if err != nil {
				errText := "Couldn't run %s on %s:"
				if output != nil {
					errText += string(output)
				}
				gplog.Error(errText, command, hostname)
				anyFailed = true
			}
		}
		if anyFailed {
			goto fail
		}

		err = h.checklistWriter.MarkComplete(upgradestatus.SEGINSTALL)
		if err != nil {
			gplog.Error(err.Error())
		}
		return

	fail:
		err = h.checklistWriter.MarkFailed(upgradestatus.SEGINSTALL)
		if err != nil {
			gplog.Error(err.Error())
		}
	}()

	// go h.remoteExecutor.VerifySoftware(h.clusterPair.GetHostnames())

	return &pb.CheckSeginstallReply{}, nil
}
