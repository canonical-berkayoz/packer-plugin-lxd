// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package lxd

import (
	"context"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
)

// StepConfigureCommunicator puts the native LXD communicator on the state bag,
// where commonsteps.StepProvision picks it up.
type StepConfigureCommunicator struct {
	comm *Communicator
}

func (s *StepConfigureCommunicator) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	config := state.Get("config").(*Config)
	client := state.Get("client").(instanceServer)

	s.comm = &Communicator{
		Client:       client,
		InstanceName: config.ContainerName,
		Environment:  config.ExecEnvironment,
		User:         config.ExecUser,
		Group:        config.ExecGroup,
		Cwd:          config.ExecCwd,
	}

	state.Put("communicator", s.comm)

	return multistep.ActionContinue
}

func (s *StepConfigureCommunicator) Cleanup(state multistep.StateBag) {
	if s.comm != nil {
		//nolint:errcheck // best-effort release of the SFTP connection
		s.comm.Close()
	}
}
