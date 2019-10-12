package services

import (
	"bytes"
	"os"
	"os/exec"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/onsi/gomega/gbytes"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
	"github.com/greenplum-db/gpupgrade/testutils/exectest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func gpinitsystem() {}

func gpinitsystem_Warnings() {
	os.Stdout.WriteString("[WARN]:-Master open file limit is 256 should be >= 65535")
	os.Exit(1)
}

func gpinitsystem_Errors() {
	os.Stderr.WriteString("[ERROR]:-Failure to init")
	os.Exit(2)
}

func init() {
	exectest.RegisterMains(
		gpinitsystem,
		gpinitsystem_Warnings,
		gpinitsystem_Errors,
	)
}

// XXX: This extra test file is needed to be in the services package and not
// services_test in order test RunInitsystemForTargetCluster() using execCommand.
func TestRunInitsystemForTargetCluster(t *testing.T) {
	g := NewGomegaWithT(t)
	ctrl := gomock.NewController(GinkgoT())
	defer ctrl.Finish()

	mockStream := mock_idl.NewMockCliToHub_ExecuteServer(ctrl)
	mockStream.EXPECT().
		Send(gomock.Any()).
		AnyTimes()

	execCommand = nil
	defer func() {
		execCommand = exec.Command
	}()

	targetBin := "/target/bin"
	gpinitsystemConfigPath := "/home/gpadmin/.gpupgrade/gpinitsystem_config"

	t.Run("uses the correct arguments", func(t *testing.T) {
		execCommand = exectest.NewCommandWithVerifier(gpinitsystem,
			func(path string, args ...string) {
				g.Expect(path).To(Equal("bash"))
				g.Expect(args).To(Equal([]string{"-c", "source /target/greenplum_path.sh && " +
					"/target/bin/gpinitsystem -a -I /home/gpadmin/.gpupgrade/gpinitsystem_config"}))
			})

		var buf bytes.Buffer
		err := RunInitsystemForTargetCluster(mockStream, &buf, targetBin, gpinitsystemConfigPath)
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("should use executables in the source's bindir even if bindir has a trailing slash", func(t *testing.T) {
		execCommand = exectest.NewCommandWithVerifier(gpinitsystem,
			func(path string, args ...string) {
				g.Expect(path).To(Equal("bash"))
				g.Expect(args).To(Equal([]string{"-c", "source /target/greenplum_path.sh && " +
					"/target/bin/gpinitsystem -a -I /home/gpadmin/.gpupgrade/gpinitsystem_config"}))
			})

		var buf bytes.Buffer
		err := RunInitsystemForTargetCluster(mockStream, &buf, targetBin + "/", gpinitsystemConfigPath)
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("when gpinitsystem has a warning it logs and does not return an error", func(t *testing.T) {
		_, _, log := testhelper.SetupTestLogger() // Store gplog output.

		execCommand = exectest.NewCommand(gpinitsystem_Warnings)

		var buf bytes.Buffer
		err := RunInitsystemForTargetCluster(mockStream, &buf, targetBin, gpinitsystemConfigPath)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(log).To((gbytes.Say("\\[WARNING\\]:-gpinitsystem had warnings and exited with status 1")))
	})

	t.Run("when gpinitsystem fails it returns an error", func(t *testing.T) {
		execCommand = exectest.NewCommand(gpinitsystem_Errors)

		var buf bytes.Buffer
		err := RunInitsystemForTargetCluster(mockStream, &buf, targetBin, gpinitsystemConfigPath)
		g.Expect(err.Error()).To(Equal("gpinitsystem failed: exit status 2"))
	})
}
