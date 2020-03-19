package hub

import (
	"database/sql"
	"fmt"
	"path/filepath"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
)

// TODO: When in copy mode should we update the catalog and in-memory object of
//  the source cluster?
func (s *Server) UpdateCatalogAndClusterConfig(streams step.OutStreams) (err error) {
	err = s.Target.StartMasterOnly(streams)
	if err != nil {
		return xerrors.Errorf("failed to start target master: %w", err)
	}

	err = WithinDbConnection(s.Target.MasterPort(), func(conn *sql.DB) error {
		return s.UpdateGpSegmentConfiguration(conn)
	})
	if err != nil {
		return xerrors.Errorf("%s: %w", idl.Substep_FINALIZE_UPDATE_TARGET_CATALOG_AND_CLUSTER_CONFIG, err)
	}

	// Create an oldTarget cluster to pass to StopMasterOnly since
	// UpdateCatalogAndClusterConfig mutates the target cluster with the new
	// data directories which have yet to be reflected on disk in a later substep.
	master := s.Target.Primaries[-1]
	master.DataDir = upgradeDataDir(master.DataDir)
	segs := map[int]greenplum.SegConfig{-1: master}
	oldTarget := &greenplum.Cluster{Primaries: segs, BinDir: s.Target.BinDir}

	err = oldTarget.StopMasterOnly(streams)
	if err != nil {
		return xerrors.Errorf("failed to stop target master: %w", err)
	}

	return nil
}

func WithinDbConnection(masterPort int, operation func(connection *sql.DB) error) (err error) {
	connURI := fmt.Sprintf("postgresql://localhost:%d/template1?gp_session_role=utility&allow_system_table_mods=true&search_path=", masterPort)
	connection, err := sql.Open("pgx", connURI)
	if err != nil {
		return xerrors.Errorf("connecting to master on port %d in utility mode with connection URI '%s': %w", masterPort, connURI, err)
	}

	defer func() {
		closeErr := connection.Close()
		if closeErr != nil {
			closeErr = xerrors.Errorf("closing connection to target master: %w", closeErr)
			err = multierror.Append(err, closeErr)
		}
	}()

	return operation(connection)
}

var ErrContentMismatch = errors.New("content ids do not match")

type ContentMismatchError struct {
	srcContents      []int
	databaseContents []int
}

func newContentMismatchError(srcContents []int, databaseContentMap map[int]bool) ContentMismatchError {
	databaseContents := []int{}
	for content := range databaseContentMap {
		databaseContents = append(databaseContents, content)
	}
	return ContentMismatchError{srcContents, databaseContents}
}

func (c ContentMismatchError) Error() string {
	return fmt.Sprintf("source content ids are %#v, database content ids are %#v",
		c.srcContents, c.databaseContents)
}

func (c ContentMismatchError) Is(err error) bool {
	return err == ErrContentMismatch
}

// contentsMatch just makes sure that the two maps (keyed by segment content ID)
// have the same keys.
//
// There's nothing magic about the map signatures here; the maps' value types
// are ignored completely.
func contentsMatch(src map[int]greenplum.SegConfig, dst map[int]bool) bool {
	for content := range src {
		if _, ok := dst[content]; !ok {
			return false
		}
	}

	for content := range dst {
		if _, ok := src[content]; !ok {
			return false
		}
	}

	return true
}

// TODO: add standby/mirrors check here too
func sanityCheckContentIDs(tx *sql.Tx, src *greenplum.Cluster) error {
	rows, err := tx.Query("SELECT content FROM gp_segment_configuration")
	if err != nil {
		return xerrors.Errorf("querying segment configuration: %w", err)
	}

	contents := make(map[int]bool)
	for rows.Next() {
		var content int
		if err := rows.Scan(&content); err != nil {
			return xerrors.Errorf("scanning segment configuration: %w", err)
		}

		contents[content] = true
	}
	if err := rows.Err(); err != nil {
		return xerrors.Errorf("iterating over segment configuration: %w", err)
	}

	if !contentsMatch(src.Primaries, contents) {
		return newContentMismatchError(src.ContentIDs, contents)
	}

	return nil
}

// commitOrRollback either Commit()s or Rollback()s the passed transaction
// depending on whether err is non-nil. It returns any error encountered during
// the operation; in the case of a rollback error, the incoming error will be
// combined with the new error in a multierror.Error.
func commitOrRollback(tx *sql.Tx, err error) error {
	if err != nil {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil {
			rollbackErr = xerrors.Errorf("rolling back transaction: %w", rollbackErr)
			err = multierror.Append(err, rollbackErr)
		}
		return err
	}

	commitErr := tx.Commit()
	if commitErr != nil {
		return xerrors.Errorf("committing transaction: %w", commitErr)
	}

	return nil
}

// UpdateGpSegmentConfiguration will modify the gp_segment_configuration of the passed
// sql.DB to match the cluster port settings from the source utils.Cluster.
//
// As a reminder to developers, we don't have any mirrors up at this point on
// the target cluster. We copy only the primary information.
func (s *Server) UpdateGpSegmentConfiguration(db *sql.DB) (err error) {
	tx, err := db.Begin()
	if err != nil {
		return xerrors.Errorf("starting transaction to update catalog: %w", err)
	}
	defer func() {
		err = commitOrRollback(tx, err)
		if err == nil {
			// After successfully changing the catalog, update the source and
			// target cluster objects to match the catalog and persist to
			// disk.
			origConf := &Config{}
			path := filepath.Join(utils.GetStateDir(), ConfigFileName)
			err = LoadConfig(origConf, path)
			if err != nil {
				err = xerrors.Errorf("loading config: %w", err)
				return
			}

			// TODO: this is out of sync now, as the standby/mirrors are added later.
			//   replace with one without standby/mirrors
			s.Target = origConf.Source
			s.Target.BinDir = origConf.Target.BinDir
			s.Target.Version = origConf.Target.Version

			err = s.SaveConfig()
		}
	}()

	// Make sure the content IDs in gp_segment_configuration match the source
	// cluster exactly.
	if err := sanityCheckContentIDs(tx, s.Source); err != nil {
		return err
	}

	// TODO: Consider iterating over dbids instead which is unique and could
	//  remove the need for specifying the role when updating the catalog.
	for _, content := range s.Source.ContentIDs {
		err := updateConfiguration(tx, s.Source.Primaries[content])
		if err != nil {
			return err
		}
	}

	return nil
}

func updateConfiguration(tx *sql.Tx, seg greenplum.SegConfig) error {
	res, err := tx.Exec("UPDATE gp_segment_configuration SET port = $1, datadir = $2 WHERE content = $3 AND role = $4",
		seg.Port, seg.DataDir, seg.ContentID, seg.Role)
	if err != nil {
		return xerrors.Errorf("updating segment configuration: %w", err)
	}

	// We should have updated only one row. More than one implies that
	// gp_segment_configuration has a primary and a mirror up for a single
	// content ID, and we can't handle mirrors at this point.
	rows, err := res.RowsAffected()
	if err != nil {
		// An error should only occur here if the driver does not support
		// this call, and we know that the postgres driver does.
		panic(fmt.Sprintf("retrieving number of rows updated: %v", err))
	}
	if rows != 1 {
		return xerrors.Errorf("updated %d rows for content %d, expected 1", rows, seg.ContentID)
	}

	return nil
}
