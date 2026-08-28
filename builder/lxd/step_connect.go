// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package lxd

import (
	"context"
	"fmt"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

// StepConnect establishes the connection to the LXD daemon that every later
// step uses.
type StepConnect struct{}

func (s *StepConnect) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	config := state.Get("config").(*Config)
	ui := state.Get("ui").(packersdk.Ui)

	ui.Say("Connecting to LXD daemon...")

	client, err := config.connectDaemon()
	if err != nil {
		return halt(state, ui, err)
	}

	server, _, err := client.GetServer()
	if err != nil {
		return halt(state, ui, fmt.Errorf("querying LXD server: %w", err))
	}

	ui.Say(fmt.Sprintf("Connected to LXD %s", server.Environment.ServerVersion))

	state.Put("client", client)

	return multistep.ActionContinue
}

func (s *StepConnect) Cleanup(state multistep.StateBag) {}

// halt records an error on the state bag and stops the build.
func halt(state multistep.StateBag, ui packersdk.Ui, err error) multistep.StepAction {
	state.Put("error", err)
	ui.Error(err.Error())

	return multistep.ActionHalt
}
