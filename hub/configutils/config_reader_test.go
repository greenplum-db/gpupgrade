package configutils_test

import (
	"encoding/json"
	"github.com/greenplum-db/gpupgrade/testutils"
	"io/ioutil"

	"github.com/greenplum-db/gpupgrade/hub/configutils"
	"regexp"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("configutils reader", func() {

	const (
		// the output is pretty-printed, so match that format precisely
		expected_json = `[
{
	"some": "json"
}
]`
	)

	var (
		configReader   configutils.Reader
		json_structure []map[string]interface{}
		dir            string
	)

	BeforeEach(func() {
		err := json.Unmarshal([]byte(expected_json), &json_structure)
		Expect(err).NotTo(HaveOccurred())

		dir, err = ioutil.TempDir("", "")
		Expect(err).ToNot(HaveOccurred())

		configReader = configutils.Reader{}
		configReader.OfOldClusterConfig(dir)
	})

	Describe("#Read", func() {
		It("reads a configuration", func() {
			testutils.WriteSampleConfig(dir)
			err := configReader.Read()

			Expect(err).NotTo(HaveOccurred())
			Expect(configReader.GetPortForSegment(7)).ToNot(BeNil())
		})

		It("returns an error if configutils cannot be read", func() {
			err := configReader.Read()
			Expect(err).To(HaveOccurred())
		})

		It("returns list of hostnames", func() {
			testutils.WriteSampleConfig(dir)
			err := configReader.Read()
			Expect(err).NotTo(HaveOccurred())
			Expect(configReader.GetHostnames()).Should(ContainElement("briarwood"))
			Expect(configReader.GetHostnames()).Should(ContainElement("aspen.pivotal"))
		})

		It("returns list of hostnames without duplicates", func() {
			re := regexp.MustCompile("aspen.pivotal")
			configWithDupe := re.ReplaceAllLiteralString(testutils.SAMPLE_JSON, "briarwood")
			testutils.WriteOldConfig(dir, configWithDupe)
			err := configReader.Read()
			Expect(err).NotTo(HaveOccurred())
			hostnames, err := configReader.GetHostnames()
			Expect(len(hostnames)).Should(Equal(1))
			Expect(configReader.GetHostnames()).Should(ContainElement("briarwood"))
		})

		It("returns its bindir", func() {
			testutils.WriteSampleConfig(dir)
			err := configReader.Read()
			Expect(err).NotTo(HaveOccurred())
			Expect(configReader.GetBinDir()).Should(ContainSubstring("/tmp"))
		})
	})
})
