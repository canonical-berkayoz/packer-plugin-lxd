// Copyright IBM Corp. 2020, 2025
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"fmt"
	"os"

	"github.com/hashicorp/packer-plugin-sdk/plugin"

	"github.com/canonical/packer-plugin-lxd/builder/lxd"
	lxdVersion "github.com/canonical/packer-plugin-lxd/version"
)

func main() {
	pps := plugin.NewSet()
	// Registered under DEFAULT_NAME so templates address the builder as
	// bare "lxd", matching the previous lxc-based plugin.
	pps.RegisterBuilder(plugin.DEFAULT_NAME, new(lxd.Builder))
	pps.SetVersion(lxdVersion.PluginVersion)
	err := pps.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
