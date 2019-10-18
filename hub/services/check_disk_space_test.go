package services_test

import (
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/pkg/errors"

	"github.com/greenplum-db/gpupgrade/hub/services"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/idl/mock_idl"
)

func TestCheckDiskSpace(t *testing.T) {
	testhelper.SetupTestLogger() // initialize gplog

	t.Run("returns err msg when unable to call CheckDiskSpaceOnAgents on segment host", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		var clients []services.ClientAndHostname

		client := mock_idl.NewMockAgentClient(ctrl)
		client.EXPECT().CheckDiskSpaceOnAgents(
			gomock.Any(),
			&idl.CheckDiskSpaceRequestToAgent{},
		).Return(&idl.CheckDiskSpaceReplyFromAgent{}, errors.New("couldn't connect to hub"))
		clients = append(clients, services.ClientAndHostname{Client: client, Hostname: "doesnotexist"})

		messages := services.GetDiskSpaceFromSegmentHosts(clients)
		if len(messages) != 1 {
			t.Fatalf("want exactly one message; returned %#v", messages)
		}

		expected := "Could not get disk usage from: "
		if !strings.Contains(messages[0], expected) {
			t.Errorf("returned %#v want prefix %#v", messages[0], expected)
		}

	})

	t.Run("lists filesystems above usage threshold", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		var clients []services.ClientAndHostname

		var expectedFilesystemsUsage []*idl.FileSysUsage
		expectedFilesystemsUsage = append(expectedFilesystemsUsage, &idl.FileSysUsage{Filesystem: "first filesystem", Usage: 90.4})
		expectedFilesystemsUsage = append(expectedFilesystemsUsage, &idl.FileSysUsage{Filesystem: "/second/filesystem", Usage: 24.2})

		client := mock_idl.NewMockAgentClient(ctrl)
		client.EXPECT().CheckDiskSpaceOnAgents(
			gomock.Any(),
			&idl.CheckDiskSpaceRequestToAgent{},
		).Return(&idl.CheckDiskSpaceReplyFromAgent{ListOfFileSysUsage: expectedFilesystemsUsage}, nil)
		clients = append(clients, services.ClientAndHostname{Client: client, Hostname: "doesnotexist"})

		messages := services.GetDiskSpaceFromSegmentHosts(clients)
		if len(messages) != 1 {
			t.Fatalf("want exactly one message; returned %#v", messages)
		}

		expected := "diskspace check - doesnotexist - WARNING first filesystem 90.4 use"
		if !strings.Contains(messages[0], expected) {
			t.Errorf("returned %#v want prefix %#v", messages[0], expected)
		}
	})

	t.Run("lists hosts for which all filesystems are below usage threshold", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		var clients []services.ClientAndHostname

		var expectedFilesystemsUsage []*idl.FileSysUsage
		expectedFilesystemsUsage = append(expectedFilesystemsUsage, &idl.FileSysUsage{Filesystem: "first filesystem", Usage: 70.4})
		expectedFilesystemsUsage = append(expectedFilesystemsUsage, &idl.FileSysUsage{Filesystem: "/second/filesystem", Usage: 24.2})

		client := mock_idl.NewMockAgentClient(ctrl)
		client.EXPECT().CheckDiskSpaceOnAgents(
			gomock.Any(),
			&idl.CheckDiskSpaceRequestToAgent{},
		).Return(&idl.CheckDiskSpaceReplyFromAgent{ListOfFileSysUsage: expectedFilesystemsUsage}, nil)
		clients = append(clients, services.ClientAndHostname{Client: client, Hostname: "doesnotexist"})

		messages := services.GetDiskSpaceFromSegmentHosts(clients)
		if len(messages) != 1 {
			t.Fatalf("want exactly one message; returned %#v", messages)
		}

		expected := "diskspace check - doesnotexist - OK"
		if !strings.Contains(messages[0], expected) {
			t.Errorf("returned %#v want prefix %#v", messages[0], expected)
		}
	})
}
