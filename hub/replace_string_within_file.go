package hub

import (
	"fmt"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/xerrors"
)

// For test stubbing purposes
// see common_test.go for stubbing function
var replaceStringWithinFileFunc = replaceStringWithinFile

//
// Use sed-like syntax to edit the text of a file:
//
// for example:
//
// sed s/pattern/replacement/ /path/to/some/file.txt
//
func ReplaceStringWithinFile(pattern string, replacement string, file string) error {
	return replaceStringWithinFileFunc(pattern, replacement, file)
}

func replaceStringWithinFile(pattern string, replacement string, pathToFile string) error {
	script := fmt.Sprintf(
		"sed 's@%[1]s@%[2]s@' %[3]s > %[3]s.updated && "+
			"mv %[3]s %[3]s.bak && "+
			"mv %[3]s.updated %[3]s",
		pattern, replacement, pathToFile)

	gplog.Debug("executing command: %+v", script) // TODO: Move this debug log into ExecuteLocalCommand()

	cmd := execCommand("bash", "-c", script)
	_, err := cmd.Output()
	if err != nil {
		return xerrors.Errorf("updating %s: %w", pathToFile, err)
	}
	return nil
}
