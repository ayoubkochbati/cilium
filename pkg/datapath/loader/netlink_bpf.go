// Copyright 2020 Authors of Cilium
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package loader

import (
	"context"
	"fmt"
	"strconv"

	"github.com/cilium/cilium/pkg/bpf"
	"github.com/cilium/cilium/pkg/command/exec"

	"github.com/cilium/ebpf"
	"github.com/vishvananda/netlink"
)

func bpfCompileProg(src, dst string, extraCFlags []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 100)
	defer cancel()

	if err := Compile(ctx, src, dst, extraCFlags); err != nil {
		return err
	}

	return nil
}

func xdpLoad(ifName, mode, src, dst, sec, cidrMap string, extraCFlags []string) error {
	link, err := netlink.LinkByName(ifName)
	if err != nil {
		return err
	}
	mac := mac2array(link.Attrs().HardwareAddr)

	extraCFlags = append(extraCFlags, fmt.Sprintf("-DNODE_MAC=%s", mac))

	if err := bpfCompileProg(src, dst, extraCFlags); err != nil {
		return err
	}

	collection, err := ebpf.LoadCollectionSpec(dst)
	if err != nil {
		return err
	}

	var retCode int64
	for _, progSpec := range collection.Programs {
		prog, err := ebpf.NewProgram(progSpec)
		if err != nil {
			return err
		}

		// TODO: mode?
		if err := netlink.LinkSetXdpFd(link, prog.FD()); err != nil {
			// return err
			retCode = 1
		}
	}

	args := []string{"-e", dst, "-r", strconv.FormatInt(retCode, 10)}
	cmd := exec.CommandContext(context.Background(), "cilium-map-migrate", args...)
	cmd.Env = bpf.Environment()
	// Ignore errors.
	cmd.CombinedOutput(log, true)

	return nil
}

func xdpUnload(ifName, mode string) error {
	// TODO
	return nil
}

func bpfLoad(ifName, where, src, dst, sec, callsMap string, extraCFlags []string) error {
	link, err := netlink.LinkByName(ifName)
	if err != nil {
		return err
	}
	mac := mac2array(link.Attrs().HardwareAddr)

	extraCFlags = append(extraCFlags, fmt.Sprintf("-DNODE_MAC=%s", mac))
	extraCFlags = append(extraCFlags, fmt.Sprintf("-DCALLS_MAP=%s", callsMap))

	if err := bpfCompileProg(src, dst, extraCFlags); err != nil {
		return err
	}

	qdisc := &netlink.Htb{
		QdiscAttrs: netlink.QdiscAttrs{
			LinkIndex: link.Attrs().Index,
		},
	}
	// Ignore error.
	netlink.QdiscReplace(qdisc)

	// [ -z "$(tc filter show dev $DEV $WHERE | grep -v 'pref 1 bpf chain 0 $\|pref 1 bpf chain 0 handle 0x1')" ] || tc filter del dev $DEV $WHERE
	// filters, err := netlink.FilterList(link, 0)
	// if err != nil {
	// 	return err
	// }
	// ffilters := make([]netlink.Filter, 0)
	// for _, filter := range filters {
	// }

	// tc filter replace dev $DEV $WHERE prio 1 handle 1 bpf da obj $OUT sec $SEC

	args := []string{"-s", dst}
	cmd := exec.CommandContext(context.Background(), "cilium-map-migrate", args...)
	cmd.Env = bpf.Environment()
	if _, err := cmd.CombinedOutput(log, true); err != nil {
		return err
	}

	return nil
}
