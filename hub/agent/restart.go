package agent

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	multierror "github.com/hashicorp/go-multierror"
	"google.golang.org/grpc"
)

var execCommand = exec.Command

func RestartAll(ctx context.Context,
	dialer func(context.Context, string) (net.Conn, error),
	hostnames []string,
	port int,
	stateDir string) ([]string, error) {

	var wg sync.WaitGroup
	restartedHosts := make(chan string, len(hostnames))
	errs := make(chan error, len(hostnames))

	for _, host := range hostnames {
		wg.Add(1)
		go func(host string) {
			defer wg.Done()

			address := host + ":" + strconv.Itoa(port)
			timeoutCtx, cancelFunc := context.WithTimeout(ctx, 3*time.Second)
			opts := []grpc.DialOption{
				grpc.WithBlock(),
				grpc.WithInsecure(),
				grpc.FailOnNonTempDialError(true),
			}
			if dialer != nil {
				opts = append(opts, grpc.WithContextDialer(dialer))
			}
			conn, err := grpc.DialContext(timeoutCtx, address, opts...)
			cancelFunc()
			if err == nil {
				err = conn.Close()
				if err != nil {
					gplog.Error("failed to close agent connection to %s: %+v", host, err)
				}
				return
			}

			gplog.Debug("failed to dial agent on %s: %+v", host, err)
			gplog.Info("starting agent on %s", host)

			agentPath, err := getAgentPath()
			if err != nil {
				errs <- err
				return
			}
			cmd := execCommand("ssh", host,
				fmt.Sprintf("bash -c \"%s agent --daemonize --state-directory %s\"", agentPath, stateDir))
			stdout, err := cmd.Output()
			if err != nil {
				errs <- err
				return
			}

			gplog.Debug(string(stdout))
			restartedHosts <- host
		}(host)
	}

	wg.Wait()
	close(errs)
	close(restartedHosts)

	var hosts []string
	for h := range restartedHosts {
		hosts = append(hosts, h)
	}

	var multiErr *multierror.Error
	for err := range errs {
		multiErr = multierror.Append(multiErr, err)
	}

	return hosts, multiErr.ErrorOrNil()
}

func getAgentPath() (string, error) {
	hubPath, err := os.Executable()
	if err != nil {
		return "", err
	}

	return filepath.Join(filepath.Dir(hubPath), "gpupgrade"), nil
}
