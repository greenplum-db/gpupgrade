package utils

import (
	"os"
	"os/user"
	"strings"
	"testing"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/pkg/errors"
	"golang.org/x/xerrors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("user utils", func() {

	AfterEach(func() {
		System = InitializeSystemFunctions()
	})

	Describe("#GetUser", func() {
		Describe("happy: when no error", func() {
			It("returns current user", func() {
				System.CurrentUser = func() (*user.User, error) {
					return &user.User{
						Username: "Joe",
						HomeDir:  "my_home_dir",
					}, nil
				}

				userName, err := GetUser()
				Expect(err).ToNot(HaveOccurred())
				Expect(userName).To(Equal("Joe"))
			})
		})
		Describe("error: when CurrentUser() fails", func() {
			It("returns an error", func() {
				System.CurrentUser = func() (*user.User, error) {
					return nil, errors.New("my deliberate user error")
				}

				_, err := GetUser()
				Expect(err).To(HaveOccurred())
			})
		})
	})
})

func TestCreateAllDataDirectories(t *testing.T) {
	testhelper.SetupTestLogger() // initialize gplog

	const dataDir = "/data/qddir_upgrade"

	t.Run("creates directory and marker if they don't already exist", func(t *testing.T) {
		defer func() {
			System = InitializeSystemFunctions()
		}()

		var marker string
		System.Stat = func(name string) (os.FileInfo, error) {
			// store the marker path for later checks
			marker = name

			// is the marker inside the data directory?
			if !strings.HasPrefix(marker, dataDir) {
				t.Errorf("want marker file %q to be in datadir %q", marker, dataDir)
			}

			return nil, os.ErrNotExist
		}

		var directoryMade, fileWritten bool

		System.Mkdir = func(path string, perm os.FileMode) error {
			if path != dataDir {
				t.Errorf("called mkdir(%q), want mkdir(%q)", path, dataDir)
			}

			directoryMade = true
			return nil
		}

		System.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			if !directoryMade {
				t.Errorf("marker file created in nonexistent data directory")
			}
			if path != marker {
				t.Errorf("marker file created at %q, want %q", path, marker)
			}
			fileWritten = true
			return nil
		}

		err := CreateDataDirectory(dataDir)
		if err != nil {
			t.Errorf("returned error %+v", err)
		}

		if !fileWritten {
			t.Error("marker file was not created")
		}
	})

	t.Run("cannot stat the master data directory", func(t *testing.T) {
		defer func() {
			System = InitializeSystemFunctions()
		}()

		expected := errors.New("permission denied")
		System.Stat = func(name string) (os.FileInfo, error) {
			return nil, expected
		}

		directoryCreated := false
		System.Mkdir = func(path string, perm os.FileMode) error {
			directoryCreated = true
			return nil
		}

		err := CreateDataDirectory(dataDir)
		if !xerrors.Is(err, expected) {
			t.Errorf("got %#v, want %#v", err, expected)
		}

		if directoryCreated {
			t.Errorf("Mkdir(%q) must not be called", dataDir)
		}
	})

	t.Run("cannot create the master data directory", func(t *testing.T) {
		defer func() {
			System = InitializeSystemFunctions()
		}()

		System.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		expected := errors.New("permission denied")
		System.Mkdir = func(path string, perm os.FileMode) error {
			return expected
		}

		err := CreateDataDirectory(dataDir)
		if !xerrors.Is(err, expected) {
			t.Errorf("got %#v, want %#v", err, expected)
		}
	})

	t.Run("data directory exist but without marker file .gpupgrade", func(t *testing.T) {
		defer func() {
			System = InitializeSystemFunctions()
		}()

		System.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		System.Mkdir = func(path string, perm os.FileMode) error {
			return os.ErrExist
		}

		directoryRemoved := false
		System.RemoveAll = func(name string) error {
			directoryRemoved = true
			return nil

		}

		expected := os.ErrExist

		err := CreateDataDirectory(dataDir)
		if !xerrors.Is(err, expected) {
			t.Errorf("got %#v, want %#v", err, expected)
		}

		if directoryRemoved {
			t.Errorf("RemoveAll(%q) must not be called", dataDir)
		}
	})

	t.Run("previous data directory is removed and new data directory is created", func(t *testing.T) {
		defer func() {
			System = InitializeSystemFunctions()
		}()

		var marker string
		System.Stat = func(name string) (os.FileInfo, error) {
			marker = name
			return nil, nil
		}

		var directoryRemoved bool
		System.RemoveAll = func(name string) error {
			directoryRemoved = true
			return nil
		}

		var directoryCreated bool
		System.Mkdir = func(path string, perm os.FileMode) error {
			if !directoryRemoved {
				t.Errorf("RemoveAll(%q) not called", dataDir)
			}

			directoryCreated = true
			return nil
		}

		var fileWritten bool
		System.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			if !directoryCreated {
				t.Errorf("Mkdir(%q, 0755) not called", dataDir)
			}

			if path != marker {
				t.Errorf("marker file created at %q, want %q", path, marker)
			}

			fileWritten = true
			return nil
		}

		err := CreateDataDirectory(dataDir)
		if err != nil {
			t.Errorf("returned error: %+v", err)
		}

		if !fileWritten {
			t.Errorf("marker file %q was not created", marker)
		}
	})
}
