// Copyright (c) 2017-2023 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package logger

import (
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"github.com/greenplum-db/gpupgrade/utils"
)

func Initialize(process string) {
	f, err := OpenFile(process)
	if err != nil {
		fmt.Printf("%+v\n", err)
		os.Exit(1)
	}

	// If more robust logging is needed consider using logutils, zap, zerolog, etc.
	log.SetOutput(f)
	log.SetPrefix(prefix())
	log.SetFlags(log.Ldate | log.Ltime | log.Lmsgprefix)
}

func OpenFile(process string) (*os.File, error) {
	logDir, err := utils.GetLogDir()
	if err != nil {
		fmt.Printf("\n%+v\n", err)
		os.Exit(1)
	}

	err = os.MkdirAll(logDir, 0755)
	if err != nil {
		fmt.Printf("\n%+v\n", err)
		os.Exit(1)
	}

	return os.OpenFile(LogPath(logDir, process), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
}

func LogPath(logDir, process string) string {
	return filepath.Join(logDir, fmt.Sprintf("%s_%s.log", process, time.Now().Format("20060102")))
}

// prefix has the form PROGRAMNAME:USERNAME:HOSTNAME:PID [LOGLEVEL]:
func prefix() string {
	currentUser, _ := user.Current()
	host, _ := os.Hostname()

	return fmt.Sprintf("gpupgrade:%s:%s:%06d [INFO]: ",
		currentUser.Username, host, os.Getpid())
}
