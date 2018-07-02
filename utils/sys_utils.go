package utils

import (
	//"github.com/jmoiron/sqlx"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"time"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/pkg/errors"
)

var (
	System = InitializeSystemFunctions()
)

/*
 * SystemFunctions holds function pointers for built-in functions that will need
 * to be mocked out for unit testing.  All built-in functions manipulating the
 * filesystem, shell, or environment should ideally be called through a function
 * pointer in System (the global SystemFunctions variable) instead of being called
 * directly.
 */

type SystemFunctions struct {
	CurrentUser     func() (*user.User, error)
	Getenv          func(key string) string
	Getpid          func() int
	Hostname        func() (string, error)
	IsNotExist      func(err error) bool
	MkdirAll        func(path string, perm os.FileMode) error
	Now             func() time.Time
	Open            func(name string) (*os.File, error)
	OpenFile        func(name string, flag int, perm os.FileMode) (*os.File, error)
	Remove          func(name string) error
	RemoveAll       func(name string) error
	ReadFile        func(filename string) ([]byte, error)
	WriteFile       func(filename string, data []byte, perm os.FileMode) error
	Stat            func(name string) (os.FileInfo, error)
	FilePathGlob    func(pattern string) ([]string, error)
	Create          func(name string) (*os.File, error)
	RunCommandAsync func(cmdStr, logFile string) error
}

func InitializeSystemFunctions() *SystemFunctions {
	return &SystemFunctions{
		CurrentUser:     user.Current,
		Getenv:          os.Getenv,
		Getpid:          os.Getpid,
		Hostname:        os.Hostname,
		IsNotExist:      os.IsNotExist,
		MkdirAll:        os.MkdirAll,
		Now:             time.Now,
		Open:            os.Open,
		OpenFile:        os.OpenFile,
		Remove:          os.Remove,
		RemoveAll:       os.RemoveAll,
		Stat:            os.Stat,
		FilePathGlob:    filepath.Glob,
		ReadFile:        ioutil.ReadFile,
		WriteFile:       ioutil.WriteFile,
		Create:          os.Create,
		RunCommandAsync: RunCommandAsync,
	}
}

func RunCommandAsync(cmdStr, logFile string) error {
	f, err := os.OpenFile(logFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		errMsg := fmt.Sprintf("mkdir %s failed: %v.", logFile, err)
		gplog.Error(errMsg)
		return errors.New(errMsg)
	}

	cmd := exec.Command("bash", "-c", cmdStr)
	cmd.Stdout = f
	cmd.Stderr = f

	err = cmd.Start()
	if err != nil {
		errMsg := fmt.Sprintf("Command %s failed to run: %s", cmdStr, err)
		gplog.Error(errMsg)
		return errors.New(errMsg)
	}
	return nil
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
