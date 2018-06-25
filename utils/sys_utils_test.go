package utils

import (
	"os/user"

	"github.com/pkg/errors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("user utils", func() {

	var saveGetenv func(string) string
	var saveCurrentUser func() (*user.User, error)
	var saveHostname func() (string, error)

	BeforeEach(func() {
		saveGetenv = System.Getenv
		saveCurrentUser = System.CurrentUser
		saveHostname = System.Hostname
	})

	AfterEach(func() {
		System.Getenv = saveGetenv
		System.CurrentUser = saveCurrentUser
		System.Hostname = saveHostname
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

})
