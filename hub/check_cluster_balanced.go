package hub

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/idl"
)

const DbidNotInBalancedStateQuery = "SELECT dbid FROM pg_catalog.gp_segment_configuration WHERE role != preferred_role"

func CheckClusterIsBalanced(sourcePort int) (err error) {
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
			idl.Substep_CHECK_CLUSTER_BALANCED, err)
	}

	dbidsSwitchedRole, err := FindUnbalancedSegments(sourceDB)
	if err != nil {
		return xerrors.Errorf("%s failed to find unbalanced segments: %w",
			idl.Substep_CHECK_CLUSTER_BALANCED, err)
	}

	if len(dbidsSwitchedRole) > 0 {
		message := fmt.Sprintf(`
		Segment dbid %s are not in balanced state.
		The cluster must be balanced for gpupgrade to continue. 
		Use gprecoverseg to rebalance the cluster.`,
			strings.Join(strings.Fields(fmt.Sprint(dbidsSwitchedRole)), ","))
		return xerrors.Errorf(message)
	}

	return nil
}

func FindUnbalancedSegments(db *sql.DB) (dbids []int, err error) {
	rows, err := db.Query(DbidNotInBalancedStateQuery)
	if err != nil {
		return nil, xerrors.Errorf("%s failed to query segment configuration: %w", idl.Substep_CHECK_CLUSTER_BALANCED, err)
	}
	defer rows.Close()

	var dbid int
	var dbidsSwitchedRole []int

	for rows.Next() {
		err = rows.Scan(&dbid)
		if err != nil {
			return nil, xerrors.Errorf("%s failed to scan rows: %w", idl.Substep_CHECK_CLUSTER_BALANCED, err)
		}
		dbidsSwitchedRole = append(dbidsSwitchedRole, dbid)
	}

	return dbidsSwitchedRole, nil
}
