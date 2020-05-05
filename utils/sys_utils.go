// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"database/sql"
	"io"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"golang.org/x/xerrors"
)

var (
	System = InitializeSystemFunctions()
)

const markerFile = ".gpupgrade"

/*
 * SystemFunctions holds function pointers for built-in functions that will need
 * to be mocked out for unit testing.  All built-in functions manipulating the
 * filesystem, shell, or environment should ideally be called through a function
 * pointer in System (the global SystemFunctions variable) instead of being called
 * directly.
 */

type SystemFunctions struct {
	CurrentUser  func() (*user.User, error)
	Getenv       func(key string) string
	Getpid       func() int
	Hostname     func() (string, error)
	IsNotExist   func(err error) bool
	MkdirAll     func(path string, perm os.FileMode) error
	Now          func() time.Time
	Open         func(name string) (*os.File, error)
	OpenFile     func(name string, flag int, perm os.FileMode) (*os.File, error)
	Remove       func(name string) error
	RemoveAll    func(name string) error
	Rename       func(oldpath, newpath string) error
	ReadFile     func(filename string) ([]byte, error)
	WriteFile    func(filename string, data []byte, perm os.FileMode) error
	Stat         func(name string) (os.FileInfo, error)
	FilePathGlob func(pattern string) ([]string, error)
	Create       func(name string) (*os.File, error)
	Mkdir        func(name string, perm os.FileMode) error
	SqlOpen      func(driverName, dataSourceName string) (*sql.DB, error)
}

func InitializeSystemFunctions() *SystemFunctions {
	return &SystemFunctions{
		CurrentUser:  user.Current,
		Getenv:       os.Getenv,
		Getpid:       os.Getpid,
		Hostname:     os.Hostname,
		IsNotExist:   os.IsNotExist,
		MkdirAll:     os.MkdirAll,
		Now:          time.Now,
		Open:         os.Open,
		OpenFile:     os.OpenFile,
		Remove:       os.Remove,
		RemoveAll:    os.RemoveAll,
		Rename:       os.Rename,
		Stat:         os.Stat,
		FilePathGlob: filepath.Glob,
		ReadFile:     ioutil.ReadFile,
		WriteFile:    ioutil.WriteFile,
		Create:       os.Create,
		Mkdir:        os.Mkdir,
		SqlOpen:      sql.Open,
	}
}

func TryEnv(varname string, defval string) string {
	val := System.Getenv(varname)
	if val == "" {
		return defval
	}
	return val
}

func GetUser() (string, string, error) {
	currentUser, err := System.CurrentUser()
	if err != nil {
		return "", "", err
	}
	return currentUser.Username, currentUser.HomeDir, err
}

func GetHost() (string, error) {
	hostname, err := System.Hostname()
	return hostname, err
}

func GetStateDir() string {
	stateDir := os.Getenv("GPUPGRADE_HOME")
	if stateDir == "" {
		stateDir = filepath.Join(os.Getenv("HOME"), ".gpupgrade")
	}

	return stateDir
}

func GetLogDir() (string, error) {
	currentUser, err := System.CurrentUser()
	if err != nil {
		return "", err
	}

	logDir := filepath.Join(currentUser.HomeDir, "gpAdminLogs", "gpupgrade")
	return logDir, nil
}

func CreateDataDirectory(dataDir string) error {
	file := filepath.Join(dataDir, markerFile)
	_, err := System.Stat(file)
	if err == nil {
		err = System.RemoveAll(dataDir)
		if err != nil {
			return xerrors.Errorf("remove data directory %s: %w", dataDir, err)
		}
	}

	if !os.IsNotExist(err) && err != nil {
		return xerrors.Errorf("stat marker file %s: %w", markerFile, err)
	}

	gplog.Info("creating directory %s", dataDir)
	err = System.Mkdir(dataDir, 0755)
	if err != nil {
		return xerrors.Errorf("create data directory %s: %w", dataDir, err)
	}

	mFile := filepath.Join(dataDir, markerFile)
	gplog.Info("creating marker file %s", mFile)
	err = System.WriteFile(mFile, []byte{}, 0644)
	if err != nil {
		return xerrors.Errorf("create gpupgrade marker file %s: %w", mFile, err)
	}
	return nil
}

// DevNull implements OutStreams by just discarding all writes.
var DevNull = devNull{}

type devNull struct{}

func (_ devNull) Stdout() io.Writer {
	return ioutil.Discard
}

func (_ devNull) Stderr() io.Writer {
	return ioutil.Discard
}

// StdStream can be passed into functions that are called
// from the CLI.
type StdStream struct {
}

func (m *StdStream) Stdout() io.Writer {
	return os.Stdout
}

func (m *StdStream) Stderr() io.Writer {
	return os.Stderr
}
