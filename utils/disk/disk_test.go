// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package disk_test

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	sigar "github.com/cloudfoundry/gosigar"
	"golang.org/x/sys/unix"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/testutils/testlog"
	"github.com/greenplum-db/gpupgrade/utils/disk"
)

func TestCheckUsage(t *testing.T) {
	testlog.SetupLogger()

	// This test disk has two mount points:
	//  - /, at 25% utilization
	//  - /tmp, at 75% utilization
	// Both are 1 MiB in size.
	// FIXME there's a bug in gosigar that prevents us from using smaller disks
	const size uint64 = 1024 * 1024
	d := testDisk{
		err: errors.New("should never happen"),

		filesystems: func() (sigar.FileSystemList, error) {
			return sigar.FileSystemList{List: []sigar.FileSystem{
				{DirName: "/"},
				{DirName: "/tmp"},
			}}, nil
		},

		usage: func(path string) (sigar.FileSystemUsage, error) {
			u := sigar.FileSystemUsage{Total: size}

			u.Avail = scale(u.Total, 0.75)
			if strings.HasPrefix(path, "/tmp") {
				u.Avail = scale(u.Total, 0.25)
			}
			u.Free = u.Avail
			u.Used = u.Total - u.Free

			return u, nil
		},

		stat: func(path string) (*unix.Stat_t, error) {
			stat := new(unix.Stat_t)

			switch {
			case strings.HasPrefix(path, "/tmp"):
				stat.Dev = 2 // tmp filesystem

			case path == "/unmounted/path":
				// Return a device ID that doesn't correspond to any filesystem
				// in the list.
				stat.Dev = 100

			default:
				stat.Dev = 1 // root filesystem
			}

			return stat, nil
		},
	}

	cases := []struct {
		name string

		ratio    float64
		paths    []string
		expected disk.SpaceFailures
	}{
		{"returns no failures with adequate space",
			0.1, []string{"/test/path"},
			disk.SpaceFailures{},
		},
		{"returns failures with inadequate space",
			0.8, []string{"/test/path"},
			disk.SpaceFailures{
				"/": &idl.CheckDiskSpaceReply_DiskUsage{
					Required:  scale(size, 0.8),
					Available: scale(size, 0.75),
				},
			},
		},
		{"returns only one failure per filesystem",
			0.8, []string{"/test/path", "/test/other/path", "/tmp/path"},
			disk.SpaceFailures{
				"/": &idl.CheckDiskSpaceReply_DiskUsage{
					Required:  scale(size, 0.8),
					Available: scale(size, 0.75),
				},
				"/tmp": &idl.CheckDiskSpaceReply_DiskUsage{
					Required:  scale(size, 0.8),
					Available: scale(size, 0.25),
				},
			},
		},
		{"uses path directly if mount point is not found",
			0.8, []string{"/unmounted/path"},
			disk.SpaceFailures{
				"/unmounted/path": &idl.CheckDiskSpaceReply_DiskUsage{
					Required:  scale(size, 0.8),
					Available: scale(size, 0.75),
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual, err := disk.CheckUsage(d, c.ratio, c.paths...)
			if err != nil {
				t.Errorf("returned error %#v", err)
			}
			if !reflect.DeepEqual(actual, c.expected) {
				t.Errorf("returned %v want %v", actual, c.expected)
			}
		})
	}

	t.Run("excludes superuser-reserved space from total disk size", func(t *testing.T) {
		// Use the previous test disk setup; just tweak the usage.
		d.usage = func(path string) (sigar.FileSystemUsage, error) {
			u := sigar.FileSystemUsage{Total: size}

			// Reserve 50% of the disk for the superuser.
			u.Avail = scale(u.Total, 0.25)
			u.Free = u.Avail + scale(u.Total, 0.50)
			u.Used = u.Total - u.Free

			return u, nil
		}

		// If you look at just the available and total disk space, you might
		// assume that we have only 25% of the disk to use. In fact we have
		// 50% of our available space, because we've only used half of the
		// unreserved disk.
		actual, err := disk.CheckUsage(d, 0.4, "/path")
		if err != nil {
			t.Errorf("returned error %#v", err)
		}
		if len(actual) > 0 {
			t.Errorf("returned %v; want no failures", actual)
		}

		// Make sure the required disk count is correctly calculated as well.
		actual, err = disk.CheckUsage(d, 0.6, "/path")
		if err != nil {
			t.Errorf("returned error %#v", err)
		}

		expected := disk.SpaceFailures{
			"/": &idl.CheckDiskSpaceReply_DiskUsage{
				Required:  scale(size/2, 0.6),
				Available: scale(size/2, 0.5),
			},
		}
		if !reflect.DeepEqual(actual, expected) {
			t.Errorf("returned %v want %v", actual, expected)
		}
	})

	// regression test to catch float representation errors
	t.Run("does floating point math correctly", func(t *testing.T) {
		d := testDisk{
			err: errors.New("should never happen"),

			filesystems: func() (sigar.FileSystemList, error) {
				return sigar.FileSystemList{List: []sigar.FileSystem{
					{DirName: "/"},
				}}, nil
			},

			// Utilization is 40% exactly.
			usage: func(path string) (sigar.FileSystemUsage, error) {
				u := sigar.FileSystemUsage{
					Total: 487260160,
					Avail: 292356096,
				}
				u.Free = u.Avail
				u.Used = u.Total - u.Avail
				return u, nil
			},

			stat: func(path string) (*unix.Stat_t, error) {
				return &unix.Stat_t{Dev: 1}, nil
			},
		}

		actual, err := disk.CheckUsage(d, 0.6, "/path")
		if err != nil {
			t.Errorf("returned error %#v", err)
		}
		if len(actual) > 0 {
			t.Errorf("returned %v; want no failures", actual)
		}
	})

	t.Run("bubbles up any errors", func(t *testing.T) {
		// This is a whitebox test. Start with the "everything fails" case, then
		// slowly implement more and more of the functionality until we've hit
		// all the error cases.
		d := testDisk{err: errors.New("the correct error")}
		checkError := func() {
			t.Helper() // don't include this function in the stack trace

			_, err := disk.CheckUsage(d, 0.5, "/test/path")
			if !errors.Is(err, d.err) {
				t.Errorf("returned %#v want %#v", err, d.err)
			}
		}

		checkError()

		d.filesystems = func() (sigar.FileSystemList, error) {
			return sigar.FileSystemList{List: []sigar.FileSystem{
				{DirName: "/"},
			}}, nil
		}
		checkError()

		d.stat = func(path string) (*unix.Stat_t, error) {
			if path == "/" {
				return &unix.Stat_t{Dev: 1}, nil
			}

			return nil, d.err
		}
		checkError()

		d.usage = func(path string) (sigar.FileSystemUsage, error) {
			return sigar.FileSystemUsage{
				Total: 1024,
				Used:  1024,
				Free:  0,
				Avail: 0,
			}, nil
		}
		checkError()

		d.stat = func(path string) (*unix.Stat_t, error) {
			return &unix.Stat_t{Dev: 1}, nil
		}

		// At this point everything should work; otherwise we're missing
		// coverage for a path.
		_, err := disk.CheckUsage(d, 0.5, "/test/path")
		if err != nil {
			t.Errorf("returned %#v after all error cases were removed", err)
		}
	})
}

func TestLocal(t *testing.T) {
	// disk.Local is a passthrough to more complicated implementations. Rather
	// than duplicate the tests for those implementations, just verify simple
	// sanity. Put deeper functionality verification into end-to-end tests.

	fs, err := disk.Local.Filesystems()
	if err != nil {
		t.Errorf("Local.Filesystems() returned error %#v", err)
	}
	if len(fs.List) < 1 {
		t.Error("Local.Filesystems() returned no entries")
	}

	dir := os.TempDir()

	usage, err := disk.Local.Usage(dir)
	if err != nil {
		t.Errorf("Local.Usage(%q) returned error %#v", dir, err)
	}
	if usage.Total == 0 {
		t.Errorf("Local.Usage(%q) returned bad usage: %+v", dir, usage)
	}

	stat, err := disk.Local.Stat(dir)
	if err != nil {
		t.Errorf("Local.Stat(%q) returned error %#v", dir, err)
	}
	if (stat.Mode & unix.S_IFDIR) == 0 {
		t.Errorf("Local.Stat(%q) did not stat a directory: %+v", dir, stat)
	}
}

// testDisk is a stub implementation of disk.Disk.
type testDisk struct {
	err error // returned whenever one of the below functions is unimplemented

	filesystems func() (sigar.FileSystemList, error)
	usage       func(string) (sigar.FileSystemUsage, error)
	stat        func(string) (*unix.Stat_t, error)
}

func (t testDisk) Filesystems() (sigar.FileSystemList, error) {
	if t.filesystems == nil {
		return sigar.FileSystemList{}, t.err
	}
	return t.filesystems()
}

func (t testDisk) Usage(path string) (sigar.FileSystemUsage, error) {
	if t.usage == nil {
		return sigar.FileSystemUsage{}, t.err
	}
	return t.usage(path)
}

func (t testDisk) Stat(path string) (*unix.Stat_t, error) {
	if t.stat == nil {
		return nil, t.err
	}
	return t.stat(path)
}

func scale(n uint64, f float64) uint64 {
	return uint64(float64(n) * f)
}
