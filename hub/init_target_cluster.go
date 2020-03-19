package hub

import (
	"fmt"
	"io/ioutil"
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
	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
)

func (s *Server) GenerateInitsystemConfig() error {
	sourceDBConn := db.NewDBConn("localhost", int(s.Source.MasterPort()), "template1")
	return s.writeConf(sourceDBConn)
}

func (s *Server) initsystemConfPath() string {
	return filepath.Join(s.StateDir, "gpinitsystem_config")
}

func (s *Server) writeConf(sourceDBConn *dbconn.DBConn) error {
	err := sourceDBConn.Connect(1)
	if err != nil {
		return errors.Wrap(err, "could not connect to database")
	}
	defer sourceDBConn.Close()

	gpinitsystemConfig, err := CreateInitialInitsystemConfig(s.Source.MasterDataDir())
	if err != nil {
		return err
	}

	gpinitsystemConfig, err = GetCheckpointSegmentsAndEncoding(gpinitsystemConfig, sourceDBConn)
	if err != nil {
		return err
	}

	gpinitsystemConfig, err = WriteSegmentArray(gpinitsystemConfig, s.TargetInitializeConfig)
	if err != nil {
		return xerrors.Errorf("generating segment array: %w", err)
	}

	return WriteInitsystemFile(gpinitsystemConfig, s.initsystemConfPath())
}

func (s *Server) CreateTargetCluster(stream step.OutStreams) error {
	err := s.InitTargetCluster(stream)
	if err != nil {
		return err
	}

	conn := db.NewDBConn("localhost", s.TargetInitializeConfig.Master.Port, "template1")
	defer conn.Close()

	s.Target, err = greenplum.ClusterFromDB(conn, s.Target.BinDir)
	if err != nil {
		return errors.Wrap(err, "could not retrieve target configuration")
	}

	if err := s.SaveConfig(); err != nil {
		return err
	}

	return nil
}

func (s *Server) InitTargetCluster(stream step.OutStreams) error {
	agentConns, err := s.AgentConns()
	if err != nil {
		return errors.Wrap(err, "Could not get/create agents")
	}

	err = CreateAllDataDirectories(agentConns, s.Source)
	if err != nil {
		return err
	}

	return RunInitsystemForTargetCluster(stream, s.Target, s.initsystemConfPath())
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

func WriteSegmentArray(config []string, targetInitializeConfig InitializeConfig) ([]string, error) {
	//Partition segments by host in order to correctly assign ports.
	if targetInitializeConfig.Master == (greenplum.SegConfig{}) {
		return nil, errors.New("source cluster contains no master segment")
	}

	master := targetInitializeConfig.Master
	config = append(config,
		fmt.Sprintf("QD_PRIMARY_ARRAY=%s~%d~%s~%d~%d~0",
			master.Hostname,
			master.Port,
			master.DataDir,
			master.DbID,
			master.ContentID,
		),
	)

	config = append(config, "declare -a PRIMARY_ARRAY=(")
	for _, segment := range targetInitializeConfig.Primaries {
		config = append(config,
			fmt.Sprintf("\t%s~%d~%s~%d~%d~0",
				segment.Hostname,
				segment.Port,
				segment.DataDir,
				segment.DbID,
				segment.ContentID,
			),
		)
	}
	config = append(config, ")")

	return config, nil
}

func CreateAllDataDirectories(agentConns []*Connection, source *greenplum.Cluster) error {
	targetDataDir := path.Dir(source.MasterDataDir()) + "_upgrade"
	err := utils.CreateDataDirectory(targetDataDir)
	if err != nil {
		return err
	}
	err = CreateSegmentDataDirectories(agentConns, source)
	if err != nil {
		return err
	}

	return nil
}

func RunInitsystemForTargetCluster(stream step.OutStreams, target *greenplum.Cluster, gpinitsystemFilepath string) error {
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

func CreateSegmentDataDirectories(agentConns []*Connection, cluster *greenplum.Cluster) error {
	wg := sync.WaitGroup{}
	errChan := make(chan error, len(agentConns))

	for _, conn := range agentConns {
		conn := conn

		// Selects all primaries belonging to this agent's host.
		primaries := func(seg *greenplum.SegConfig) bool {
			return seg.Hostname == conn.Hostname &&
				(seg.Role == greenplum.PrimaryRole && seg.ContentID != -1)
		}

		wg.Add(1)
		go func(c *Connection) {
			defer wg.Done()

			segments := cluster.SelectSegments(primaries)
			if len(segments) == 0 {
				// This can happen if a host contains only a standby and/or
				// mirrors.
				return
			}

			req := new(idl.CreateSegmentDataDirRequest)
			for _, seg := range segments {
				// gpinitsystem needs the *parent* directories of the new
				// segment data directories to exist.
				datadir := filepath.Dir(upgradeDataDir(seg.DataDir))
				req.Datadirs = append(req.Datadirs, datadir)
			}

			_, err := c.AgentClient.CreateSegmentDataDirectories(context.Background(), req)
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
