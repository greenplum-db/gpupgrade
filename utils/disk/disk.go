// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package disk

import (
	"golang.org/x/sys/unix"
	"golang.org/x/xerrors"

	sigar "github.com/cloudfoundry/gosigar"
	"github.com/greenplum-db/gp-common-go-libs/gplog"

	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/step"
	"github.com/greenplum-db/gpupgrade/utils"
)

type Disk interface {
	Filesystems() (sigar.FileSystemList, error)
	Usage(string) (sigar.FileSystemUsage, error)
	Stat(string) (*unix.Stat_t, error)
}

type FileSystemDiskUsage []*idl.CheckDiskSpaceReply_DiskUsage

func (f FileSystemDiskUsage) Len() int {
	return len(f)
}

func (f FileSystemDiskUsage) Less(i, j int) bool {
	fi, fj := f[i], f[j]

	if fi.GetHost() == fj.GetHost() {
		return fi.GetFs() < fj.GetFs()
	}
	return fi.GetHost() < fj.GetHost()
}

func (f FileSystemDiskUsage) Swap(i, j int) {
	f[i], f[j] = f[j], f[i]
}

// CheckUsage uses the given Disk to look up filesystem usage for each path, and
// compares the available space to the required disk ratio. Any filesystems that
// don't have enough space will be given an entry in the returned SpaceFailures
// map. Note that this is one entry per filesystem, not one entry per path.
//
// This function ignores space that has been reserved for the superuser (i.e.
// the difference between "free" and "avail" in statfs(2)). It does not consider
// that space to be free for use, nor does it count that space against the total
// disk size. For example, a disk with 25% avail space and 75% free space -- as
// defined by statfs(2) -- would be considered 50% available by CheckUsage.
func CheckUsage(streams step.OutStreams, d Disk, diskFreeRatio float64, paths ...string) (FileSystemDiskUsage, error) {
	hostname, err := utils.System.Hostname()
	if err != nil {
		return nil, xerrors.Errorf("determining hostname: %w", err)
	}

	failures := make(map[string]*idl.CheckDiskSpaceReply_DiskUsage)

	// Find the device ID for every filesystem. We'll use these to map data
	// directories to filesystems later.
	fs, err := d.Filesystems()
	if err != nil {
		return nil, xerrors.Errorf("enumerating filesystems: %w", err)
	}

	fsByID := make(map[uint64]string)
	for _, f := range fs.List {
		stat, err := d.Stat(f.DirName)
		if err != nil {
			return nil, xerrors.Errorf("stat'ing %s: %w", f.DirName, err)
		}

		fsByID[uint64(stat.Dev)] = f.DirName
	}

	for _, path := range paths {
		usage, err := d.Usage(path)
		if err != nil {
			return nil, xerrors.Errorf("getting fs usage for %s: %w", path, err)
		}

		// Exclude superuser-reserved space.
		total := usage.Used + usage.Avail
		required := uint64(diskFreeRatio * float64(total))

		gplog.Debug("%s: %d avail of %d required (%d used, %d total)",
			path, usage.Avail, required, usage.Used, usage.Total)

		if usage.Avail < required {
			// Get the filesystem that this path belongs to.
			stat, err := d.Stat(path)
			if err != nil {
				return nil, xerrors.Errorf("stat'ing %s: %w", path, err)
			}

			fs, ok := fsByID[uint64(stat.Dev)]
			if !ok {
				// Rather than blow up if we can't associate a path with a
				// filesystem, just use the path itself.
				fs = path
			}

			failures[fs] = &idl.CheckDiskSpaceReply_DiskUsage{
				Fs:        fs,
				Host:      hostname,
				Required:  required,
				Available: usage.Avail,
			}
		}
	}

	// NOTE: Transform the failures map used to prevent duplicate filesystems
	// into a list since protobuf can't handle maps with complex keys such as a
	// struct of filesystem and host.
	var usage FileSystemDiskUsage
	for _, failure := range failures {
		usage = append(usage, failure)
	}

	return usage, nil
}

// Local is a standard implementation of the Disk interface that uses gosigar
// and unix.Stat to obtain statistics for the local machine.
var Local = local{}

type local struct{}

func (_ local) Filesystems() (sigar.FileSystemList, error) {
	var list sigar.FileSystemList
	err := list.Get()
	return list, err
}

func (_ local) Usage(path string) (sigar.FileSystemUsage, error) {
	var usage sigar.FileSystemUsage
	err := usage.Get(path)
	return usage, err
}

func (_ local) Stat(path string) (*unix.Stat_t, error) {
	stat := new(unix.Stat_t)
	err := unix.Stat(path, stat)
	return stat, err
}
