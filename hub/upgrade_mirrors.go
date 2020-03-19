package hub

import (
	"database/sql"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/utils"
)

const defaultFTSTimeout = 2 * time.Minute

func writeGpAddmirrorsConfig(mirrors []greenplum.SegConfig, out io.Writer) error {
	for _, m := range mirrors {
		_, err := fmt.Fprintf(out, "%d|%s|%d|%s\n", m.ContentID, m.Hostname, m.Port, m.DataDir)
		if err != nil {
			return err
		}
	}
	return nil
}

func runAddMirrors(r greenplum.Runner, filepath string) error {
	return r.Run("gpaddmirrors",
		"-a",
		"-i", filepath,
	)
}

func waitForFTS(db *sql.DB, timeout time.Duration) error {
	startTime := time.Now()
	for {
		rows, err := db.Query("SELECT gp_request_fts_probe_scan();")
		if err != nil {
			return xerrors.Errorf("requesting probe scan: %w", err)
		}

		if err := rows.Close(); err != nil {
			return xerrors.Errorf("closing probe scan results: %w", err)
		}

		doneWaiting, err := func() (bool, error) {
			var up bool
			rows, err = db.Query(`
				SELECT every(status = 'u' AND mode = 's')
					FROM gp_segment_configuration
					WHERE role = 'm'
			`)
			if err != nil {
				return false, xerrors.Errorf("querying mirror status: %w", err)
			}

			defer rows.Close() // XXX lost error

			for rows.Next() {
				if err := rows.Scan(&up); err != nil {
					return false, xerrors.Errorf("scanning mirror status: %w", err)
				}
			}
			if err := rows.Err(); err != nil {
				return false, xerrors.Errorf("iterating mirror status: %w", err)
			}

			return up, nil
		}()

		if err != nil {
			return err
		}

		if doneWaiting {
			return nil
		}

		if time.Since(startTime) > timeout {
			return xerrors.Errorf("%s timeout exceeded waiting for mirrors to come up", timeout)
		}

		time.Sleep(time.Second)
	}
}

func UpgradeMirrors(stateDir string, masterPort int, mirrors []greenplum.SegConfig, targetRunner greenplum.Runner) (err error) {
	connURI := fmt.Sprintf("postgresql://localhost:%d/template1?gp_session_role=utility&search_path=", masterPort)
	db, err := utils.System.SqlOpen("pgx", connURI)
	if err != nil {
		return err
	}

	defer db.Close()

	return doUpgrade(db, stateDir, mirrors, targetRunner)
}

func doUpgrade(db *sql.DB, stateDir string, mirrors []greenplum.SegConfig, targetRunner greenplum.Runner) (err error) {
	path := filepath.Join(stateDir, "add_mirrors_config")
	// calling Close() on a file twice results in an error
	// only call Close() in the defer if we haven't yet tried to close it.
	fileClosed := false

	f, err := utils.System.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if !fileClosed {
			if cerr := f.Close(); cerr != nil {
				err = multierror.Append(err, cerr).ErrorOrNil()
			}
		}
	}()

	err = writeGpAddmirrorsConfig(mirrors, f)
	if err != nil {
		return err
	}

	err = f.Close()
	fileClosed = true
	// not unit tested because stubbing it properly
	// would require too many extra layers
	if err != nil {
		return err
	}

	err = runAddMirrors(targetRunner, path)
	if err != nil {
		return err
	}

	return waitForFTS(db, defaultFTSTimeout)
}
