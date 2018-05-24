package cluster_ssher

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/greenplum-db/gpupgrade/helpers"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
)

type ClusterSsher struct {
	checklistWriter ChecklistWriter
	AgentPinger     AgentPinger
	commandExecer   helpers.CommandExecer
}

type ChecklistWriter interface {
	MarkInProgress(string) error
	ResetStateDir(string) error
	MarkFailed(string) error
	MarkComplete(string) error
}

type AgentPinger interface {
	PingPollAgents() error
}

func NewClusterSsher(cw ChecklistWriter, ap AgentPinger, commandExecer helpers.CommandExecer) *ClusterSsher {
	return &ClusterSsher{
		checklistWriter: cw,
		AgentPinger:     ap,
		commandExecer:   commandExecer,
	}
}

func (c *ClusterSsher) VerifySoftware(hostnames []string) {
	agentPath := filepath.Join(os.Getenv("GPHOME"), "bin", "gpupgrade_agent")
	statedir := "seginstall"
	anyFailed := c.remoteExec(hostnames, statedir, []string{"ls", agentPath})
	handleStatusLogging(c, statedir, anyFailed)
}

func (c *ClusterSsher) Start(hostnames []string) {
	// ssh -o "StrictHostKeyChecking=no" hostname /path/to/gpupgrade_agent
	statedir := "start-agents"
	gphome := os.Getenv("GPHOME")
	agentPath := filepath.Join(gphome, "bin", "gpupgrade_agent")
	greenplumPath := filepath.Join(gphome, "greenplum_path.sh")
	////ssh -n -f user@host "sh -c 'cd /whereever; nohup ./whatever > /dev/null 2>&1 &'"
	completeCommandString := fmt.Sprintf(`sh -c '. %s ; nohup %s > /dev/null 2>&1 & '`, greenplumPath, agentPath)
	c.remoteExec(hostnames, statedir, []string{completeCommandString})

	//check that all the agents are running
	var err error
	err = c.AgentPinger.PingPollAgents()
	anyFailed := err != nil
	handleStatusLogging(c, statedir, anyFailed)
}

func (c *ClusterSsher) remoteExec(hostnames []string, statedir string, command []string) bool {
	err := c.checklistWriter.ResetStateDir(statedir)
	if err != nil {
		gplog.Error(err.Error())
		//For MMVP, return here, but maybe should log more info
		return true
	}
	err = c.checklistWriter.MarkInProgress(statedir)
	if err != nil {
		gplog.Error(err.Error())
		//For MMVP, return here, but maybe should log more info
		return true
	}
	//default assumption: GPDB is installed on the same path on all hosts in cluster
	//we're looking for gpupgrade_agent as proof that the new binary is installed
	//TODO: if this finds nothing, should we err out? do a fallback check based on $GPHOME?
	var anyFailed = false
	for _, hostname := range hostnames {
		sshArgs := []string{"-o", "StrictHostKeyChecking=no", hostname}
		sshArgs = append(sshArgs, command...)
		output, err := c.commandExecer("ssh", sshArgs...).CombinedOutput()
		if err != nil {
			errText := "Couldn't run %s on %s:"
			if output != nil {
				errText += string(output)
			}
			gplog.Error(errText, command, hostname)
			anyFailed = true
		}
	}
	return anyFailed
}

func handleStatusLogging(c *ClusterSsher, statedir string, anyFailed bool) {
	if anyFailed {
		err := c.checklistWriter.MarkFailed(statedir)
		if err != nil {
			gplog.Error(err.Error())
		}
		return
	}
	err := c.checklistWriter.MarkComplete(statedir)
	if err != nil {
		gplog.Error(err.Error())
	}
}
