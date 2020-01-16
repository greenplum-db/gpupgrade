// The insert_target_config utility inserts the configuration of the target GPDB
// cluster into the specified <configPath> file.
// The GPDB cluster is identified by the $PGPORT environment variable.
// The usage is:
//
//     insert_target_config <binDir> <configPath>
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
	// open the file to overwrite the existing contents
	file, err := os.OpenFile(configPath, os.O_RDWR, 0)
	if err != nil {
		log.Fatal(err)
	}

	defer file.Close()

	var config hub.Config
	// load the contents of the file to config
	err = config.Load(file)
	if err != nil {
		log.Fatal(err)
	}

	// populate the contents of target cluster to config
	conn := dbconn.NewDBConnFromEnvironment("postgres")
	config.Target, err = utils.ClusterFromDB(conn, binDir)
	if err != nil {
		log.Fatal(err)
	}

	// go to the start of the file
	_, err = file.Seek(0, io.SeekStart)
	if err != nil {
		log.Fatal(err)
	}

	// truncate the file so that the new config could be
	// written
	err = file.Truncate(0)
	if err != nil {
		log.Fatal(err)
	}

	// write the new contents to the file
	err = config.Save(file)
	if err != nil {
		log.Fatal(err)
	}
}
