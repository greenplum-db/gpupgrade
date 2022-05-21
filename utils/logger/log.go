// Copyright (c) 2022 VMware, Inc. or its affiliates
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

	// If more robust logging is needed consider using the logrus package.
	log.SetOutput(f)
	log.SetPrefix(prefix())
	log.SetFlags(0)
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

	path := filepath.Join(logDir, fmt.Sprintf("%s_%s.log", process, timestamp(false)))
	return os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
}

// prefix has the form PROGRAMNAME:USERNAME:HOSTNAME:PID-[LOGLEVEL]:-
func prefix() string {
	currentUser, _ := user.Current()
	host, _ := os.Hostname()

	return fmt.Sprintf("%s gpupgrade:%s:%s:%06d-[INFO]:-",
		timestamp(true),
		currentUser.Username, host, os.Getpid())
}

func timestamp(includeTime bool) string {
	layout := "20060102"
	if includeTime {
		layout += ":15:04:05"
	}

	return time.Now().Format(layout)
}
