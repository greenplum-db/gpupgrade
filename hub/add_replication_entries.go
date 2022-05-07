// Copyright (c) 2017-2022 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package hub

import (
	"context"
	"net"

	"github.com/greenplum-db/gpupgrade/greenplum"
	"github.com/greenplum-db/gpupgrade/idl"
	"github.com/greenplum-db/gpupgrade/utils"
)

func AddReplicationEntriesOnPrimaries(agentConns []*idl.Connection, intermediate *greenplum.Cluster, useHbaHostnames bool) error {
	user, err := utils.System.Current()
	if err != nil {
		return err
	}

	request := func(conn *idl.Connection) error {
		intermediatePrimaries := intermediate.SelectSegments(func(seg *greenplum.SegConfig) bool {
			return seg.IsOnHost(conn.Hostname) && !seg.IsCoordinator() && seg.IsPrimary()
		})

		var entries []*idl.AddReplicationEntriesRequest_Entry
		for _, intermediatePrimary := range intermediatePrimaries {
			intermediateMirror := intermediate.Mirrors[intermediatePrimary.ContentID]

			mirrorHostAddrs := []string{intermediateMirror.Hostname}
			if useHbaHostnames {
				err, mirrorIps := getIpAddresses(intermediateMirror.Hostname)
				if err != nil {
					return err
				}

				mirrorHostAddrs = mirrorIps
			}

			conf := &idl.AddReplicationEntriesRequest_Entry{
				DataDir:   intermediatePrimary.DataDir,
				User:      user.Username,
				HostAddrs: mirrorHostAddrs,
			}

			entries = append(entries, conf)
		}

		req := &idl.AddReplicationEntriesRequest{Entries: entries}
		_, err := conn.AgentClient.AddReplicationEntries(context.Background(), req)
		return err
	}

	return ExecuteRPC(agentConns, request)
}

// getIpAddresses returns a list of ip addresses with CIDR notation for use in
// pg_hba.conf.
func getIpAddresses(host string) (error, []string) {
	ips, err := utils.System.LookupIP(host)
	if err != nil {
		return err, nil
	}

	var cidrs []string
	for _, ip := range ips {
		cidr := net.IPNet{IP: ip, Mask: net.CIDRMask(32, 32)}
		if ip.To16() != nil {
			cidr = net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}
		}

		cidrs = append(cidrs, cidr.String())
	}

	return nil, cidrs
}
