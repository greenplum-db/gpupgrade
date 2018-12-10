package upgradestatus_test

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/greenplum-db/gpupgrade/hub/upgradestatus"
	"github.com/greenplum-db/gpupgrade/hub/upgradestatus/file"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("upgradestatus/ChecklistManager", func() {
	AfterEach(func() {
		utils.System = utils.InitializeSystemFunctions()
	})

	Describe("MarkInProgress", func() {
		var (
			tempdir string
			cm      *upgradestatus.ChecklistManager
		)

		BeforeEach(func() {
			var err error
			tempdir, err = ioutil.TempDir("", "")
			Expect(err).NotTo(HaveOccurred())

			cm = upgradestatus.NewChecklistManager(filepath.Join(tempdir, ".gpupgrade"))
		})

		AfterEach(func() {
			os.RemoveAll(tempdir)
		})

		It("Leaves an in-progress file in the state dir", func() {
			step := cm.GetStepWriter("fancy_step")
			step.ResetStateDir()
			err := step.MarkInProgress()
			Expect(err).ToNot(HaveOccurred())
			expectedFile := filepath.Join(tempdir, ".gpupgrade", "fancy_step", file.InProgress)
			_, err = os.Stat(expectedFile)
			Expect(err).ToNot(HaveOccurred())
		})

		It("still succeeds if file already exists", func() {
			step := cm.GetStepWriter("fancy_step")
			step.ResetStateDir()
			step.MarkInProgress() // lay the file down once
			err := step.MarkInProgress()
			Expect(err).ToNot(HaveOccurred())
			expectedFile := filepath.Join(tempdir, ".gpupgrade", "fancy_step", file.InProgress)
			_, err = os.Stat(expectedFile)
			Expect(err).ToNot(HaveOccurred())
		})

		It("errors if file opening fails, e.g. disk full", func() {
			utils.System.OpenFile = func(_ string, _ int, _ os.FileMode) (*os.File, error) {
				return nil, errors.New("Disk full or something")
			}

			step := cm.GetStepWriter("fancy_step")
			step.ResetStateDir()
			err := step.MarkInProgress()
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("ResetStateDir", func() {
		It("errors if existing files cant be deleted", func() {
			utils.System.RemoveAll = func(name string) error {
				return errors.New("cant remove all")
			}
			cm := upgradestatus.NewChecklistManager("/some/random/dir")
			step := cm.GetStepWriter("fancy_step")
			err := step.ResetStateDir()
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
			step := cm.GetStepWriter("fancy_step")
			err := step.ResetStateDir()
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
			step := cm.GetStepWriter("fancy_step")
			err := step.ResetStateDir()
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("MarkFailed", func() {
		It("errors if in.progress file can't be removed", func() {
			utils.System.Remove = func(string) error {
				return errors.New("remove failed")
			}
			cm := upgradestatus.NewChecklistManager("/some/random/dir")
			step := cm.GetStepWriter("step")
			err := step.MarkFailed()
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
			step := cm.GetStepWriter("step")
			err := step.MarkFailed()
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
			step := cm.GetStepWriter("step")
			err := step.MarkFailed()
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("MarkComplete", func() {
		It("errors if in.progress file can't be removed", func() {
			utils.System.Remove = func(string) error {
				return errors.New("remove failed")
			}
			cm := upgradestatus.NewChecklistManager("/some/random/dir")
			step := cm.GetStepWriter("step")
			err := step.MarkFailed()
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
			step := cm.GetStepWriter("step")
			err := step.MarkComplete()
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
			step := cm.GetStepWriter("step")
			err := step.MarkComplete()
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("AddWritableStep", func() {
		It("adds a step that can be written and read", func() {
			tempdir, err := ioutil.TempDir("", "")
			Expect(err).NotTo(HaveOccurred())
			defer os.RemoveAll(tempdir)

			cm := upgradestatus.NewChecklistManager(filepath.Join(tempdir, ".gpupgrade"))
			cm.AddWritableStep("my-step", 0)

			writer := cm.GetStepWriter("my-step")
			writer.ResetStateDir()
			writer.MarkInProgress()
			writer.MarkComplete()

			reader := cm.GetStepReader("my-step")
			Expect(reader.Status()).To(Equal(idl.StepStatus_COMPLETE))
		})

		It("adds a step that is retrievable via AllSteps", func() {
			tempdir, err := ioutil.TempDir("", "")
			Expect(err).NotTo(HaveOccurred())
			defer os.RemoveAll(tempdir)

			cm := upgradestatus.NewChecklistManager(filepath.Join(tempdir, ".gpupgrade"))
			cm.AddWritableStep("my-step", 0)

			allSteps := cm.AllSteps()
			Expect(allSteps).To(HaveLen(1))
			Expect(allSteps[0].Name()).To(Equal("my-step"))
		})

		It("panics if a step with the same name has already been added", func() {
			cm := upgradestatus.NewChecklistManager("some/random/dir")
			cm.AddWritableStep("my-step", 0)
			Expect(func() { cm.AddWritableStep("my-step", 0) }).To(Panic())
		})
	})

	Describe("AddReadOnlyStep", func() {
		It("adds a step that can be read using the given status func", func() {
			cm := upgradestatus.NewChecklistManager("some/random/dir")
			cm.AddReadOnlyStep("my-step", 0, func(name string) idl.StepStatus {
				Expect(name).To(Equal("my-step"))
				return idl.StepStatus_FAILED
			})

			reader := cm.GetStepReader("my-step")
			Expect(reader.Status()).To(Equal(idl.StepStatus_FAILED))
		})

		It("adds a step that cannot be written", func() {
			cm := upgradestatus.NewChecklistManager("some/random/dir")
			cm.AddReadOnlyStep("my-step", 0, func(string) idl.StepStatus {
				return idl.StepStatus_COMPLETE
			})

			Expect(func() { cm.GetStepWriter("my-step") }).To(Panic())
		})

		It("adds a step that is retrievable via AllSteps", func() {
			cm := upgradestatus.NewChecklistManager("some/random/dir")
			cm.AddReadOnlyStep("my-step", 0, func(string) idl.StepStatus {
				return idl.StepStatus_COMPLETE
			})

			allSteps := cm.AllSteps()
			Expect(allSteps).To(HaveLen(1))
			Expect(allSteps[0].Name()).To(Equal("my-step"))
		})

		It("panics if a step with the same name has already been added", func() {
			cm := upgradestatus.NewChecklistManager("some/random/dir")
			statusFunc := func(string) idl.StepStatus {
				return idl.StepStatus_COMPLETE
			}

			cm.AddReadOnlyStep("my-step", 0, statusFunc)
			Expect(func() { cm.AddReadOnlyStep("my-step", 0, statusFunc) }).To(Panic())
		})
	})
})
