package hub

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/greenplum-db/gp-common-go-libs/cluster"
	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/db"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

func (h *Hub) GenerateInitsystemConfig(ports []uint32) (int, error) {
	sourceDBConn := db.NewDBConn("localhost", int(h.Source.MasterPort()), "template1")
	return h.writeConf(sourceDBConn, ports)
}

func (h *Hub) initsystemConfPath() string {
	return filepath.Join(h.StateDir, "gpinitsystem_config")
}

func (h *Hub) writeConf(sourceDBConn *dbconn.DBConn, ports []uint32) (int, error) {
	err := sourceDBConn.Connect(1)
	if err != nil {
		return 0, errors.Wrap(err, "could not connect to database")
	}
	defer sourceDBConn.Close()

	gpinitsystemConfig, err := CreateInitialInitsystemConfig(h.Source.MasterDataDir())
	if err != nil {
		return 0, err
	}

	gpinitsystemConfig, err = GetCheckpointSegmentsAndEncoding(gpinitsystemConfig, sourceDBConn)
	if err != nil {
		return 0, err
	}

	gpinitsystemConfig, masterPort, err := WriteSegmentArray(gpinitsystemConfig, h.Source, ports)
	if err != nil {
		return 0, xerrors.Errorf("generating segment array: %w", err)
	}

	return masterPort, WriteInitsystemFile(gpinitsystemConfig, h.initsystemConfPath())
}

func (h *Hub) CreateTargetCluster(stream OutStreams, masterPort int) error {
	err := h.InitTargetCluster(stream)
	if err != nil {
		return err
	}

	conn := db.NewDBConn("localhost", masterPort, "template1")
	defer conn.Close()

	h.Target, err = utils.ClusterFromDB(conn, h.Target.BinDir)
	if err != nil {
		return errors.Wrap(err, "could not retrieve target configuration")
	}

	if err := h.SaveConfig(); err != nil {
		return err
	}

	return nil
}

func (h *Hub) InitTargetCluster(stream OutStreams) error {
	agentConns, err := h.AgentConns()
	if err != nil {
		return errors.Wrap(err, "Could not get/create agents")
	}

	err = CreateAllDataDirectories(agentConns, h.Source)
	if err != nil {
		return err
	}

	return RunInitsystemForTargetCluster(stream, h.Target, h.initsystemConfPath())
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

func upgradeDataDir(path string) string {
	// e.g.
	//   /data/primary/seg1
	// becomes
	//   /data/primary_upgrade/seg1
	path = filepath.Clean(path)
	parent := fmt.Sprintf("%s_upgrade", filepath.Dir(path))
	return filepath.Join(parent, filepath.Base(path))
}

// sanitize sorts and deduplicates a slice of port numbers.
func sanitize(ports []uint32) []uint32 {
	sort.Slice(ports, func(i, j int) bool { return ports[i] < ports[j] })

	dedupe := ports[:0] // point at the same backing array

	var last uint32
	for i, port := range ports {
		if i == 0 || port != last {
			dedupe = append(dedupe, port)
		}
		last = port
	}

	return dedupe
}

func WriteSegmentArray(config []string, source *utils.Cluster, ports []uint32) ([]string, int, error) {
	// Partition segments by host in order to correctly assign ports.
	segmentsByHost := make(map[string][]cluster.SegConfig)
	for _, content := range source.ContentIDs {
		if content == -1 {
			continue
		}
		segment := source.Segments[content]
		segmentsByHost[segment.Hostname] = append(segmentsByHost[segment.Hostname], segment)
	}

	if len(ports) == 0 {
		// Create a default port range, starting with the pg_upgrade default of
		// 50432. Reserve enough ports to handle the host with the most
		// segments.
		var maxSegs int
		for _, segments := range segmentsByHost {
			if len(segments) > maxSegs {
				maxSegs = len(segments)
			}
		}

		// Add 1 for the reserved master port
		for i := 0; i < maxSegs+1; i++ {
			ports = append(ports, uint32(50432+i))
		}
	}

	ports = sanitize(ports)
	masterPort := ports[0]
	segmentPorts := ports[1:]

	// Use a copy of the source cluster's segment configs rather than modifying
	// the source cluster. This keeps the in-memory representation of source
	// cluster consistent with its on-disk representation.
	copySegments := make(map[int]cluster.SegConfig)
	for _, segments := range segmentsByHost {
		if len(segmentPorts) < len(segments) {
			return nil, 0, errors.New("not enough ports for each segment")
		}

		for i, segment := range segments {
			segment.Port = int(segmentPorts[i])
			copySegments[segment.ContentID] = segment
		}
	}

	master, ok := source.Segments[-1]
	if !ok {
		return nil, 0, errors.New("old cluster contains no master segment")
	}

	config = append(config,
		fmt.Sprintf("QD_PRIMARY_ARRAY=%s~%d~%s~%d~%d~0",
			master.Hostname,
			masterPort,
			upgradeDataDir(master.DataDir),
			master.DbID,
			master.ContentID,
		),
	)

	config = append(config, "declare -a PRIMARY_ARRAY=(")
	for _, content := range source.ContentIDs {
		if content == -1 {
			continue
		}

		segment := copySegments[content]
		config = append(config,
			fmt.Sprintf("\t%s~%d~%s~%d~%d~0",
				segment.Hostname,
				segment.Port,
				upgradeDataDir(segment.DataDir),
				segment.DbID,
				segment.ContentID,
			),
		)
	}
	config = append(config, ")")

	return config, int(masterPort), nil
}

func CreateAllDataDirectories(agentConns []*Connection, source *utils.Cluster) error {
	// create master data directory for gpinitsystem if it doesn't exist
	targetDataDir := path.Dir(source.MasterDataDir()) + "_upgrade"
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
	err = CreateSegmentDataDirectories(agentConns, source)
	if err != nil {
		return xerrors.Errorf("segment data directories: %w", err)
	}
	return nil
}

func RunInitsystemForTargetCluster(stream OutStreams, target *utils.Cluster, gpinitsystemFilepath string) error {
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

	cmd.Stdout = stream.Stdout()
	cmd.Stderr = stream.Stderr()

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

func CreateSegmentDataDirectories(agentConns []*Connection, cluster *utils.Cluster) error {
	wg := sync.WaitGroup{}
	errChan := make(chan error, len(agentConns))

	for _, conn := range agentConns {
		wg.Add(1)

		go func(c *Connection) {
			defer wg.Done()

			segments, err := cluster.SegmentsOn(c.Hostname)
			if err != nil {
				errChan <- err
				return
			}

			req := new(idl.CreateSegmentDataDirRequest)
			for _, seg := range segments {
				// gpinitsystem needs the *parent* directories of the new
				// segment data directories to exist.
				datadir := filepath.Dir(upgradeDataDir(seg.DataDir))
				req.Datadirs = append(req.Datadirs, datadir)
			}

			_, err = c.AgentClient.CreateSegmentDataDirectories(context.Background(), req)
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
