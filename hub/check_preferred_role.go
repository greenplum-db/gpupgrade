package hub

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/idl"
)

func CheckSegmentNotInPreferredRole(sourcePort int) error {
	connURI := fmt.Sprintf("postgresql://localhost:%d/template1?gp_session_role=utility&search_path=", sourcePort)
	sourceDB, err := sql.Open("pgx", connURI)
	defer func() {
		closeErr := sourceDB.Close()
		if closeErr != nil {
			closeErr = xerrors.Errorf("closing connection to new master db: %w", closeErr)
			err = multierror.Append(err, closeErr)
		}
	}()
	if err != nil {
		return xerrors.Errorf("%s failed to open connection to utility master: %w",
			idl.Substep_CHECK_PREFERRED_ROLE, err)
	}

	return CheckDbIdsNotInPreferredRole(sourceDB)
}

func CheckDbIdsNotInPreferredRole(db *sql.DB) error {
	rows, err := db.Query(`SELECT dbid
								FROM pg_catalog.gp_segment_configuration
								WHERE role != preferred_role`)
	if err != nil {
		return xerrors.Errorf("%s failed to query segment configuration: %w", idl.Substep_CHECK_PREFERRED_ROLE, err)
	}
	defer rows.Close()

	var dbid int
	var dbidsSwitchedRole []int

	for rows.Next() {
		err = rows.Scan(&dbid)
		if err != nil {
			xerrors.Errorf("%s failed to scan rows: %w", idl.Substep_CHECK_PREFERRED_ROLE, err)
		}
		dbidsSwitchedRole = append(dbidsSwitchedRole, dbid)
	}

	if len(dbidsSwitchedRole) > 0 {
		message := fmt.Sprintf(`
		Segment dbid %s are not in preferred role.
		All the segments must be in their preferred role for gpupgrade to continue. 
		Use gprecoverseg for bringing the segments in their preferred role.`,
			strings.Join(strings.Fields(fmt.Sprint(dbidsSwitchedRole)), ","))
		return xerrors.Errorf(message)
	}

	return nil
}
