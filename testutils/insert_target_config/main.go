// The dump_config utility dumps the configuration of a running GPDB
// cluster into the specified <configPath> file.
// The GPDB cluster is identified by the $PGPORT environment variable.
// The usage is:
//
//     dump_config <binDir> <configPath>
//
// where <binDir> is what you want the configuration to contain for
// the binary location.
package main

import (
	"io"
	"log"
	"os"

	"github.com/greenplum-db/gp-common-go-libs/dbconn"

	"github.com/greenplum-db/gpupgrade/hub"
	"github.com/greenplum-db/gpupgrade/utils"
)

func main() {
	if len(os.Args) != 3 {
		log.Fatalf("usage: %s <binDir> <configPath>", os.Args[0])
	}

	binDir := os.Args[1]
	configPath := os.Args[2]
	file, err := os.OpenFile(configPath, os.O_RDWR, 0)
	if err != nil {
		log.Fatal(err)
	}

	defer file.Close()

	var config hub.PersistedConfig
	err = config.Load(file)
	if err != nil {
		log.Fatal(err)
	}

	conn := dbconn.NewDBConnFromEnvironment("postgres")
	config.Target, err = utils.ClusterFromDB(conn, binDir)
	if err != nil {
		log.Fatal(err)
	}

	_, err = file.Seek(0, io.SeekStart)
	if err != nil {
		log.Fatal(err)
	}

	err = file.Truncate(0)
	if err != nil {
		log.Fatal(err)
	}

	err = config.Save(file)
	if err != nil {
		log.Fatal(err)
	}
}
