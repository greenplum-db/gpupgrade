package hub

import (
	"database/sql"
	"fmt"
	"os/exec"

	"github.com/pkg/errors"

	"github.com/greenplum-db/gpupgrade/idl"

	"github.com/greenplum-db/gpupgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"
)

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
func contentsMatch(src map[int]utils.SegConfig, dst map[int]bool) bool {
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

func sanityCheckContentIDs(tx *sql.Tx, src *utils.Cluster) error {
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

// clonePortsFromCluster will modify the gp_segment_configuration of the passed
// sql.DB to match the cluster port settings from the source utils.Cluster.
//
// As a reminder to developers, we don't have any mirrors up at this point on
// the target cluster. We copy only the primary information. Good thing too,
// because utils.Cluster doesn't give us mirror info.
func ClonePortsFromCluster(db *sql.DB, src *utils.Cluster) (err error) {
	tx, err := db.Begin()
	if err != nil {
		return xerrors.Errorf("starting transaction for port clone: %w", err)
	}
	defer func() {
		err = commitOrRollback(tx, err)
	}()

	// Make sure the content IDs in gp_segment_configuration match the source
	// cluster exactly.
	if err := sanityCheckContentIDs(tx, src); err != nil {
		return err
	}

	for _, content := range src.ContentIDs {
		port := src.Primaries[content].Port
		res, err := tx.Exec("UPDATE gp_segment_configuration SET port = $1 WHERE content = $2",
			port, content)
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
			return xerrors.Errorf("updated %d rows for content %d, expected 1", rows, content)
		}
	}

	return nil
}

func UpdateCatalogWithPortInformation(source, target *utils.Cluster) error {
	connURI := fmt.Sprintf("postgresql://localhost:%d/template1?gp_session_role=utility&allow_system_table_mods=true&search_path=", target.MasterPort())
	targetDB, err := sql.Open("pgx", connURI)
	defer func() {
		closeErr := targetDB.Close()
		if closeErr != nil {
			closeErr = xerrors.Errorf("closing connection to new master db: %w", closeErr)
			err = multierror.Append(err, closeErr)
		}
	}()
	if err != nil {
		return xerrors.Errorf("%s failed to open connection to utility master: %w",
			idl.Substep_FINALIZE_UPDATE_CATALOG_WITH_PORT, err)
	}
	err = ClonePortsFromCluster(targetDB, source)
	if err != nil {
		return xerrors.Errorf("%s failed to clone ports: %w",
			idl.Substep_FINALIZE_UPDATE_CATALOG_WITH_PORT, err)
	}

	return nil
}

func UpdateMasterPostgresqlConf(source, target *utils.Cluster) error {
	script := fmt.Sprintf(
		"sed 's/port=%d/port=%d/' %[3]s/postgresql.conf > %[3]s/postgresql.conf.updated && "+
			"mv %[3]s/postgresql.conf %[3]s/postgresql.conf.bak && "+
			"mv %[3]s/postgresql.conf.updated %[3]s/postgresql.conf",
		target.MasterPort(), source.MasterPort(), target.MasterDataDir(),
	)
	gplog.Debug("executing command: %+v", script) // TODO: Move this debug log into ExecuteLocalCommand()
	cmd := exec.Command("bash", "-c", script)
	_, err := cmd.Output()
	if err != nil {
		return xerrors.Errorf("%s failed to execute sed command: %w",
			idl.Substep_FINALIZE_UPDATE_POSTGRESQL_CONF, err)
	}
	return nil
}
