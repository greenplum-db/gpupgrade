package services

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/db"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

func (h *Hub) CreateTargetCluster(stream messageSender, log io.Writer) error {
	sourceDBConn := db.NewDBConn("localhost", int(h.source.MasterPort()), "template1")

	targetDBConn, err := h.InitTargetCluster(stream, log, sourceDBConn)
	if err != nil {
		return errors.Wrap(err, "failed to connect to old database")
	}

	return ReloadAndCommitCluster(h.target, targetDBConn)
}

func (h *Hub) InitTargetCluster(stream messageSender, log io.Writer, sourceDBConn *dbconn.DBConn) (*dbconn.DBConn, error) {
	err := sourceDBConn.Connect(1)
	if err != nil {
		return nil, errors.Wrap(err, "could not connect to database")
	}
	defer sourceDBConn.Close()

	gpinitsystemConfig, err := CreateInitialInitsystemConfig(h.source.MasterDataDir())
	if err != nil {
		return nil, err
	}

	gpinitsystemConfig, err = GetCheckpointSegmentsAndEncoding(gpinitsystemConfig, sourceDBConn)
	if err != nil {
		return nil, err
	}

	agentConns := []*Connection{}
	agentConns, err = h.AgentConns()
	if err != nil {
		return nil, errors.Wrap(err, "Could not get/create agents")
	}

	gpinitsystemConfig, segmentDataDirMap, targetPort := DeclareDataDirectories(gpinitsystemConfig, *h.source)
	err = CreateAllDataDirectories(agentConns, segmentDataDirMap, h.source.MasterDataDir())
	if err != nil {
		return nil, err
	}

	gpinitsystemFilepath := filepath.Join(h.conf.StateDir, "gpinitsystem_config")
	err = WriteInitsystemFile(gpinitsystemConfig, gpinitsystemFilepath)
	if err != nil {
		return nil, err
	}

	err = RunInitsystemForTargetCluster(stream, log, h.target, gpinitsystemFilepath)
	if err != nil {
		return nil, err
	}

	targetDBConn := db.NewDBConn("localhost", targetPort, "template1")
	return targetDBConn, nil
}

func GetCheckpointSegmentsAndEncoding(gpinitsystemConfig []string, dbConnector *dbconn.DBConn) ([]string, error) {
	checkpointSegments, err := dbconn.SelectString(dbConnector, "SELECT current_setting('checkpoint_segments') AS string")
	if err != nil {
		return gpinitsystemConfig, errors.Wrap(err, "Could not retrieve checkpoint segments")
	}
	encoding, err := dbconn.SelectString(dbConnector, "SELECT current_setting('server_encoding') AS string")
	if err != nil {
		return gpinitsystemConfig, errors.Wrap(err, "Could not retrieve server encoding")
	}
	gpinitsystemConfig = append(gpinitsystemConfig,
		fmt.Sprintf("CHECK_POINT_SEGMENTS=%s", checkpointSegments),
		fmt.Sprintf("ENCODING=%s", encoding))
	return gpinitsystemConfig, nil
}

func CreateInitialInitsystemConfig(sourceMasterDataDir string) ([]string, error) {
	gpinitsystemConfig := []string{`ARRAY_NAME="gp_upgrade cluster"`}

	segPrefix, err := GetMasterSegPrefix(sourceMasterDataDir)
	if err != nil {
		return gpinitsystemConfig, errors.Wrap(err, "Could not get master segment prefix")
	}

	gplog.Info("Data Dir: %s", sourceMasterDataDir)
	gplog.Info("segPrefix: %v", segPrefix)
	gpinitsystemConfig = append(gpinitsystemConfig, "SEG_PREFIX="+segPrefix, "TRUSTED_SHELL=ssh")

	return gpinitsystemConfig, nil
}

func WriteInitsystemFile(gpinitsystemConfig []string, gpinitsystemFilepath string) error {
	gpinitsystemContents := []byte(strings.Join(gpinitsystemConfig, "\n"))

	err := ioutil.WriteFile(gpinitsystemFilepath, gpinitsystemContents, 0644)
	if err != nil {
		return errors.Wrap(err, "Could not write gpinitsystem_config file")
	}
	return nil
}

func DeclareDataDirectories(gpinitsystemConfig []string, source utils.Cluster) ([]string, map[string][]string, int) {
	// declare master data directory
	master := source.Segments[-1]
	master.Port++
	master.DataDir = fmt.Sprintf("%s_upgrade/%s", path.Dir(master.DataDir), path.Base(master.DataDir))
	datadirDeclare := fmt.Sprintf("QD_PRIMARY_ARRAY=%s~%d~%s~%d~%d~0",
		master.Hostname, master.Port, master.DataDir, master.DbID, master.ContentID)
	gpinitsystemConfig = append(gpinitsystemConfig, datadirDeclare)
	// declare segment data directories
	segmentDataDirMap := map[string][]string{}
	segmentDeclarations := []string{}
	for _, content := range source.ContentIDs {
		if content != -1 {
			segment := source.Segments[content]
			// FIXME: Arbitrary assumption.	 Do something smarter later
			segment.Port += 4000
			datadir := fmt.Sprintf("%s_upgrade", path.Dir(segment.DataDir))
			segment.DataDir = fmt.Sprintf("%s/%s", datadir, path.Base(segment.DataDir))
			segmentDataDirMap[segment.Hostname] = append(segmentDataDirMap[segment.Hostname],
				datadir)
			declaration := fmt.Sprintf("\t%s~%d~%s~%d~%d~0",
				segment.Hostname, segment.Port, segment.DataDir, segment.DbID, segment.ContentID)
			segmentDeclarations = append(segmentDeclarations, declaration)
		}
	}
	datadirDeclare = fmt.Sprintf("declare -a PRIMARY_ARRAY=(\n%s\n)", strings.Join(segmentDeclarations, "\n"))
	gpinitsystemConfig = append(gpinitsystemConfig, datadirDeclare)
	return gpinitsystemConfig, segmentDataDirMap, master.Port
}

func CreateAllDataDirectories(agentConns []*Connection, segmentDataDirMap map[string][]string, sourceMasterDataDir string) error {
	// create master data directory for gpinitsystem if it doesn't exist
	targetDataDir := path.Dir(sourceMasterDataDir) + "_upgrade"
	_, err := utils.System.Stat(targetDataDir)
	if os.IsNotExist(err) {
		err = utils.System.MkdirAll(targetDataDir, 0755)
		if err != nil {
			return xerrors.Errorf("master upgrade directory %s: %w", targetDataDir, err)
		}
	} else if err != nil {
		return xerrors.Errorf("stat master upgrade directory %s: %w", targetDataDir, err)
	}
	// create segment data directories for gpinitsystem if they don't exist
	err = CreateSegmentDataDirectories(agentConns, segmentDataDirMap)
	if err != nil {
		return xerrors.Errorf("segment data directories: %w", err)
	}
	return nil
}

func RunInitsystemForTargetCluster(stream messageSender, log io.Writer, target *utils.Cluster, gpinitsystemFilepath string) error {
	gphome := filepath.Dir(path.Clean(target.BinDir)) //works around https://github.com/golang/go/issues/4837 in go10.4

	args := "-a -I " + gpinitsystemFilepath
	if target.Version.SemVer.Major < 7 {
		// For 6X we add --ignore-warnings to gpinitsystem to return 0 on
		// warnings and 1 on errors. 7X and later does this by default.
		args += " --ignore-warnings"
	}

	script := fmt.Sprintf("source %[1]s/greenplum_path.sh && %[1]s/bin/gpinitsystem %[2]s",
		gphome,
		args,
	)
	cmd := execCommand("bash", "-c", script)

	mux := newMultiplexedStream(stream, log)
	cmd.Stdout = mux.NewStreamWriter(idl.Chunk_STDOUT)
	cmd.Stderr = mux.NewStreamWriter(idl.Chunk_STDERR)

	err := cmd.Run()
	if err != nil {
		return xerrors.Errorf("gpinitsystem: %w", err)
	}

	return nil
}

func GetMasterSegPrefix(datadir string) (string, error) {
	const masterContentID = "-1"

	base := path.Base(datadir)
	if !strings.HasSuffix(base, masterContentID) {
		return "", fmt.Errorf("path requires a master content identifier: '%s'", datadir)
	}

	segPrefix := strings.TrimSuffix(base, masterContentID)
	if segPrefix == "" {
		return "", fmt.Errorf("path has no segment prefix: '%s'", datadir)
	}
	return segPrefix, nil
}

func CreateSegmentDataDirectories(agentConns []*Connection, dataDirMap map[string][]string) error {
	wg := sync.WaitGroup{}
	errChan := make(chan error, len(agentConns))

	for _, conn := range agentConns {
		wg.Add(1)

		go func(c *Connection) {
			defer wg.Done()

			_, err := c.AgentClient.CreateSegmentDataDirectories(context.Background(),
				&idl.CreateSegmentDataDirRequest{
					Datadirs: dataDirMap[c.Hostname],
				})
			if err != nil {
				gplog.Error("Error creating segment data directories on host %s: %s",
					c.Hostname, err.Error())
				errChan <- err
			}
		}(conn)
	}

	wg.Wait()
	close(errChan)

	// TODO: Use a multierror to differentiate errors between hosts.
	for err := range errChan {
		if err != nil {
			return xerrors.Errorf("segment data directories: %w", err)
		}
	}
	return nil
}
