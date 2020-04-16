package upgrade

import (
	"fmt"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/xerrors"

	"github.com/greenplum-db/gpupgrade/utils"
)

const OldSuffix = "_old"

// pgUpgradeDirectory returns a path to a directory underneath the state
// directory that is to be used for storing pg_upgrade state. It does not ensure
// the directory exists first.
func pgUpgradeDirectory(stateDir string) string {
	return filepath.Join(stateDir, "pg_upgrade")
}

// SegmentWorkingDirectory returns a path to a directory underneath the
// pgUpgradeDirectory that is to be used for pg_upgrade state for a given
// segment. Like pgUpgradeDirectory, it does not ensure that the directory
// exists.
func SegmentWorkingDirectory(stateDir string, contentID int) string {
	return filepath.Join(pgUpgradeDirectory(stateDir), fmt.Sprintf("seg%d", contentID))
}

// MasterWorkingDirectory is a convenience method equivalent to
// SegmentWorkingDirectory(stateDir, -1).
func MasterWorkingDirectory(stateDir string) string {
	return SegmentWorkingDirectory(stateDir, -1)
}

// TempDataDir transforms a data directory into a corresponding temporary path
// suitable for an upgrade target, using the desired cluster segment prefix and
// upgrade ID for uniqification.
//
// The rules are currently as follows due to 6X gpinitsystem requirements:
//
// - The temporary datadir will be placed next to the original datadir.
//
// - If the datadir basename starts with the segment prefix, the remainder of
// the basename is considered the segment suffix. The temporary datadir will
// also start with the segment prefix and end with the segment suffix.
//
// - If the datadir basename does not start with the segment prefix (as can
// happen with e.g. standby data directories), the temporary datadir will
// start with the original basename.
func TempDataDir(datadir, segPrefix string, id ID) string {
	datadir = filepath.Clean(datadir) // sanitize trailing slashes for Split
	dir, base := filepath.Split(datadir)

	var newBase string
	if strings.HasPrefix(base, segPrefix) {
		suffix := strings.TrimPrefix(base, segPrefix)
		newBase = fmt.Sprintf("%s.%s.%s", segPrefix, id, suffix)
	} else {
		newBase = fmt.Sprintf("%s.%s", base, id)
	}

	return filepath.Join(dir, newBase)
}

// renames source to archive, and target to source
// e.g. for source /data/dbfast1/demoDataDir0 becomes /data/dbfast1/demoDataDir0_old
// e.g. for target /data/dbfast1/demoDataDir.123ABC.0 becomes /data/dbfast1/demoDataDir0
func RenameDataDirectory(source, archive, target string, renameTarget bool) error {
	if alreadyRenamed(archive, target) {
		return nil
	}

	err := utils.System.Rename(source, archive)
	if err != nil {
		if !xerrors.Is(err, syscall.ENOENT) {
			return err
		}

		gplog.Debug("Renaming '%q' to '%q'. Source directory does not exist. It was already renamed from a previous re-run.", source, archive)
	}

	if !renameTarget {
		return nil
	}

	err = utils.System.Rename(target, source)
	if err != nil {
		return err
	}

	return nil
}

func alreadyRenamed(archive, target string) bool {
	return PathExists(archive) && !PathExists(target)
}

func PathExists(path string) bool {
	_, err := utils.System.Stat(path)
	return err == nil
}
