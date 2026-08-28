// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package lxd

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"testing"

	"github.com/hashicorp/packer-plugin-sdk/acctest"
)

//go:embed test-fixtures/template.pkr.hcl
var testBuilderAccBasic string

// Requires PACKER_ACC=1, a reachable LXD daemon, and the plugin installed via
// `make dev`.
func TestAccLXDBuilder(t *testing.T) {
	testCase := &acctest.PluginTestCase{
		Name: "lxd_builder_basic_test",
		Teardown: func() error {
			// The build publishes an image; remove it so reruns start clean.
			cmd := exec.Command("lxc", "image", "delete", "packer-lxd-acc-test")
			_ = cmd.Run()

			return nil
		},
		Template: testBuilderAccBasic,
		Type:     "lxd",
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState != nil {
				if buildCommand.ProcessState.ExitCode() != 0 {
					return fmt.Errorf("bad exit code: %d; logfile: %s",
						buildCommand.ProcessState.ExitCode(), logfile)
				}
			}

			logs, err := os.ReadFile(logfile)
			if err != nil {
				return fmt.Errorf("reading logfile %s: %w", logfile, err)
			}

			// The image fingerprint must come back from the publish operation.
			matched, _ := regexp.Match(`Created image: [0-9a-f]{12,}`, logs)
			if !matched {
				return fmt.Errorf("no published image fingerprint in logs; logfile: %s", logfile)
			}

			return nil
		},
	}

	acctest.TestPlugin(t, testCase)
}
