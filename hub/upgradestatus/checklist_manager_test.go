package upgradestatus_test

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("upgradestatus/ChecklistManager", func() {
	AfterEach(func() {
		utils.System = utils.InitializeSystemFunctions()
	})

	Describe("MarkInProgress", func() {
		It("Leaves an in-progress file in the state dir", func() {
			tempdir, _ := ioutil.TempDir("", "")

			cm := upgradestatus.NewChecklistManager(filepath.Join(tempdir, ".gpupgrade"))
			cm.ResetStateDir("fancy_step")
			err := cm.MarkInProgress("fancy_step")
			Expect(err).ToNot(HaveOccurred())
			expectedFile := filepath.Join(tempdir, ".gpupgrade", "fancy_step", "in.progress")
			_, err = os.Stat(expectedFile)
			Expect(err).ToNot(HaveOccurred())
		})

		It("still succeeds if file already exists", func() {
			tempdir, _ := ioutil.TempDir("", "")

			cm := upgradestatus.NewChecklistManager(filepath.Join(tempdir, ".gpupgrade"))
			cm.ResetStateDir("fancy_step")
			cm.MarkInProgress("fancy_step") // lay the file down once
			err := cm.MarkInProgress("fancy_step")
			Expect(err).ToNot(HaveOccurred())
			expectedFile := filepath.Join(tempdir, ".gpupgrade", "fancy_step", "in.progress")
			_, err = os.Stat(expectedFile)
			Expect(err).ToNot(HaveOccurred())
		})

		It("errors if file opening fails, e.g. disk full", func() {
			utils.System.OpenFile = func(_ string, _ int, _ os.FileMode) (*os.File, error) {
				return nil, errors.New("Disk full or something")
			}

			tempdir, _ := ioutil.TempDir("", "")

			cm := upgradestatus.NewChecklistManager(filepath.Join(tempdir, ".gpupgrade"))
			cm.ResetStateDir("fancy_step")
			err := cm.MarkInProgress("fancy_step")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("ResetStateDir", func() {
		It("errors if existing files cant be deleted", func() {
			utils.System.RemoveAll = func(name string) error {
				return errors.New("cant remove all")
			}
			cm := upgradestatus.NewChecklistManager("/some/random/dir")
			err := cm.ResetStateDir("fancy_step")
			Expect(err).To(HaveOccurred())
		})

		It("errors if making the directory fails", func() {
			utils.System.RemoveAll = func(name string) error {
				return nil
			}
			utils.System.MkdirAll = func(string, os.FileMode) error {
				return errors.New("cant make dir")
			}
			cm := upgradestatus.NewChecklistManager("/some/random/dir")
			err := cm.ResetStateDir("fancy_step")
			Expect(err).To(HaveOccurred())
		})
		It("succeeds as long as we assume the file system calls do their job", func() {
			utils.System.RemoveAll = func(name string) error {
				return nil
			}
			utils.System.MkdirAll = func(string, os.FileMode) error {
				return nil
			}
			cm := upgradestatus.NewChecklistManager("/some/random/dir")
			err := cm.ResetStateDir("fancy_step")
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("MarkFailed", func() {
		It("errors if in.progress file can't be removed", func() {
			utils.System.Remove = func(string) error {
				return errors.New("remove failed")
			}
			cm := upgradestatus.NewChecklistManager("/some/random/dir")
			err := cm.MarkFailed("step")
			Expect(err.Error()).To(ContainSubstring("remove failed"))
		})
		It("errors if failed file can't be created", func() {
			utils.System.Remove = func(string) error {
				return nil
			}
			utils.System.OpenFile = func(string, int, os.FileMode) (*os.File, error) {
				return nil, errors.New("open file failed")
			}
			cm := upgradestatus.NewChecklistManager("/some/random/dir")
			err := cm.MarkFailed("step")
			Expect(err.Error()).To(ContainSubstring("open file failed"))
		})
		It("returns nil if nothing fails", func() {
			utils.System.Remove = func(string) error {
				return nil
			}
			utils.System.OpenFile = func(string, int, os.FileMode) (*os.File, error) {
				return nil, nil
			}
			cm := upgradestatus.NewChecklistManager("/some/random/dir")
			err := cm.MarkFailed("step")
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("MarkComplete", func() {
		It("errors if in.progress file can't be removed", func() {
			utils.System.Remove = func(string) error {
				return errors.New("remove failed")
			}
			cm := upgradestatus.NewChecklistManager("/some/random/dir")
			err := cm.MarkFailed("step")
			Expect(err.Error()).To(ContainSubstring("remove failed"))
		})
		It("errors if completed file can't be created", func() {
			utils.System.Remove = func(string) error {
				return nil
			}
			utils.System.OpenFile = func(string, int, os.FileMode) (*os.File, error) {
				return nil, errors.New("open file failed")
			}
			cm := upgradestatus.NewChecklistManager("/some/random/dir")
			err := cm.MarkComplete("step")
			Expect(err.Error()).To(ContainSubstring("open file failed"))
		})
		It("returns nil if nothing fails", func() {
			utils.System.Remove = func(string) error {
				return nil
			}
			utils.System.OpenFile = func(string, int, os.FileMode) (*os.File, error) {
				return nil, nil
			}
			cm := upgradestatus.NewChecklistManager("/some/random/dir")
			err := cm.MarkComplete("step")
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
