package utils

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"

	"github.com/pkg/errors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("user utils", func() {

	AfterEach(func() {
		System = InitializeSystemFunctions()
	})

	Describe("#TryEnv", func() {
		Describe("happy: when an environmental variable exists", func() {
			It("returns the value", func() {
				System.Getenv = func(s string) string {
					return "foo"
				}

				rc := TryEnv("bar", "mydefault")
				Expect(rc).To(Equal("foo"))
			})
		})
		Describe("error: when an environmental variable does not exist", func() {
			It("returns the default value", func() {
				System.Getenv = func(s string) string {
					return ""
				}

				rc := TryEnv("bar", "mydefault")
				Expect(rc).To(Equal("mydefault"))
			})
		})
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

				userName, userDir, err := GetUser()
				Expect(err).ToNot(HaveOccurred())
				Expect(userName).To(Equal("Joe"))
				Expect(userDir).To(Equal("my_home_dir"))
			})
		})
		Describe("error: when CurrentUser() fails", func() {
			It("returns an error", func() {
				System.CurrentUser = func() (*user.User, error) {
					return nil, errors.New("my deliberate user error")
				}

				_, _, err := GetUser()
				Expect(err).To(HaveOccurred())
			})
		})
	})
	Describe("#GetHost", func() {
		Describe("happy: when no error", func() {
			It("returns host", func() {
				System.Hostname = func() (string, error) {
					return "my_host", nil
				}

				hostname, err := GetHost()
				Expect(err).ToNot(HaveOccurred())
				Expect(hostname).To(Equal("my_host"))
			})
		})
		Describe("error: when Hostname() fails", func() {
			It("returns an error", func() {
				System.Hostname = func() (string, error) {
					return "", errors.New("my deliberate hostname error")
				}

				_, err := GetHost()
				Expect(err).To(HaveOccurred())
			})
		})

	})

	Describe("#WriteJSONFile", func() {
		var (
			dir      string
			fileName string
		)

		BeforeEach(func() {
			var err error
			dir, err = ioutil.TempDir("", "")
			Expect(err).ToNot(HaveOccurred())

			fileName = dir + "/upgrade_settings.json"
		})

		AfterEach(func() {
			err := os.RemoveAll(dir)
			Expect(err).ToNot(HaveOccurred())
		})

		readJSON := func(fileName string) map[string]string {
			_, err := os.Open(fileName)
			Expect(err).ToNot(HaveOccurred())

			contents, err := System.ReadFile(fileName)
			Expect(err).ToNot(HaveOccurred())

			result := make(map[string]string)
			err = json.Unmarshal([]byte(contents), &result)
			Expect(err).ToNot(HaveOccurred())

			return result
		}

		It("writes a map to a json file", func() {
			expected := map[string]string{}
			expected["someFlag"] = "some-value"

			err := WriteJSONFile(fileName, expected)
			Expect(err).ToNot(HaveOccurred())

			result := readJSON(fileName)
			Expect(expected).To(Equal(result))
		})

		It("writes map to a existing file, correctly truncating old contents", func() {
			f, err := os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			Expect(err).ToNot(HaveOccurred())

			brotherSays := `...And Saint Attila raised the hand grenade up on high,
			saying, "O LORD, bless this Thy hand grenade that with it Thou mayest
			blow Thine enemies to tiny bits, in Thy mercy." And the LORD did grin and
			the people did feast upon the lambs and sloths and carp and anchovies and
			orangutans and breakfast cereals, and fruit bats and large chu ---`

			_, err = f.Write([]byte(brotherSays))
			Expect(err).ToNot(HaveOccurred())

			err = f.Close()
			Expect(err).ToNot(HaveOccurred())

			expected := map[string]string{}
			expected["someFlag"] = "some-value"

			err = WriteJSONFile(fileName, expected)
			Expect(err).ToNot(HaveOccurred())

			result := readJSON(fileName)
			Expect(result).To(Equal(expected))
		})

		It("fails when passing an object that can't be json.Marshal()'d", func() {
			badStruct := map[interface{}]string{}
			key := struct{}{}
			badStruct[key] = "dummy_val"

			err := WriteJSONFile(fileName, badStruct)
			Expect(err).To(HaveOccurred())
		})

		// XXX: This is an implementation specific regression test
		It("doesn't remove temp file if exists and write fails", func() {
			tempFileName := fileName + ".tmp"

			f, err := os.OpenFile(tempFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			Expect(err).ToNot(HaveOccurred())

			err = f.Close()
			Expect(err).ToNot(HaveOccurred())

			failedUpdate := map[string]string{}
			failedUpdate["someFlag"] = "updated-value"

			System.WriteFile = func(_ string, _ []byte, _ os.FileMode) error {
				return errors.New("Mock write failed")
			}
			err = WriteJSONFile(fileName, failedUpdate)
			Expect(err).To(HaveOccurred())

			_, err = os.Stat(tempFileName)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("GetStateDir()", func() {
		It("correctly uses sync.Once and latches the value on the first call", func() {

			var stateDir_1 string
			{ // the first call to GetStateDir() sets the value to GPUPGRADE_HOME
				home_1, err := ioutil.TempDir("", "GetStateDir_")
				Expect(err).ToNot(HaveOccurred())

				oldStateDir, isSet := os.LookupEnv("GPUGRADE_HOME")
				defer func() {
					if isSet {
						os.Setenv("GPUPGRADE_HOME", oldStateDir)
					} else {
						os.Unsetenv("GPUPGRADE_HOME")
					}
				}()
				stateDir_1 = filepath.Join(home_1, ".stateDir_1")
				err = os.Setenv("GPUPGRADE_HOME", stateDir_1)
				Expect(err).ToNot(HaveOccurred())

				curStateDir := GetStateDir()
				Expect(curStateDir).To(Equal(stateDir_1))
			}

			{ // the second call to GetStateDir() preserves the value of the first call
				home_2, err := ioutil.TempDir("", "GetStateDir_")
				Expect(err).ToNot(HaveOccurred())
				stateDir_2 := filepath.Join(home_2, ".stateDir_2")
				err = os.Setenv("GPUPGRADE_HOME", stateDir_2)
				Expect(err).ToNot(HaveOccurred())

				curStateDir := GetStateDir()
				Expect(curStateDir).To(Equal(stateDir_1))
				Expect(curStateDir).ToNot(Equal(stateDir_2))
			}
		})
	})

	Describe("getStateDirInternal()", func() {

		origStateDir, isSet := os.LookupEnv("GPUGRADE_HOME")
		defer func() {
			if isSet {
				os.Setenv("GPUPGRADE_HOME", origStateDir)
			} else {
				os.Unsetenv("GPUPGRADE_HOME")
			}
		}()

		It("uses GPUPGRADE_HOME instead of HOME if set", func() {

			home, err := ioutil.TempDir("", "")
			Expect(err).ToNot(HaveOccurred())

			_, isSet := os.LookupEnv("HOME")
			Expect(isSet).To(Equal(true))

			expectedDir := filepath.Join(home, ".gpupgrade")
			err = os.Setenv("GPUPGRADE_HOME", expectedDir)
			Expect(err).ToNot(HaveOccurred())

			actualDir := getStateDirInternal()
			Expect(expectedDir).To(Equal(actualDir))

		})

		It("uses HOME if GPUPGRADE_HOME is not set", func() {

			curHOME, isSet := os.LookupEnv("HOME")
			Expect(isSet).To(Equal(true))

			err := os.Unsetenv("GPUPGRADE_HOME")
			Expect(err).ToNot(HaveOccurred())

			expectedDir := filepath.Join(curHOME, ".gpupgrade")

			actualDir := getStateDirInternal()
			Expect(expectedDir).To(Equal(actualDir))

		})
	})
})
