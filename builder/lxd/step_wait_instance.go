// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package lxd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/canonical/lxd/shared/api"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

// pollInterval is how often readiness is re-checked while waiting.
const pollInterval = time.Second

// StepWaitInstance blocks until the instance can actually run commands.
//
// This replaces the lxc-based plugin's blind `init_sleep`: instead of guessing
// that three seconds is enough for /tmp and networking to come up, it waits for
// the instance to report Running and then for an exec probe to succeed. For
// virtual machines that also waits for lxd-agent, since exec only works once
// the agent is up.
type StepWaitInstance struct{}

func (s *StepWaitInstance) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	config := state.Get("config").(*Config)
	ui := state.Get("ui").(packersdk.Ui)
	client := state.Get("client").(instanceServer)

	ui.Say("Waiting for instance to become ready...")

	ctx, cancel := context.WithTimeout(ctx, config.BootTimeout)
	defer cancel()

	if err := waitForRunning(ctx, client, config.ContainerName); err != nil {
		return halt(state, ui, err)
	}

	if err := waitForExec(ctx, client, config.ContainerName); err != nil {
		return halt(state, ui, err)
	}

	if config.WaitForNetwork {
		ui.Say("Waiting for instance to get an IPv4 address...")

		ip, err := waitForNetwork(ctx, client, config.ContainerName)
		if err != nil {
			return halt(state, ui, err)
		}

		ui.Say(fmt.Sprintf("Instance address: %s", ip))
		state.Put("instance_ip", ip)
	}

	ui.Say("Instance is ready.")

	return multistep.ActionContinue
}

func (s *StepWaitInstance) Cleanup(state multistep.StateBag) {}

// waitForRunning waits until LXD reports the instance as Running.
func waitForRunning(ctx context.Context, client instanceServer, name string) error {
	return poll(ctx, fmt.Sprintf("instance %s to start", name), func() (bool, error) {
		state, _, err := client.GetInstanceState(name)
		if err != nil {
			return false, nil
		}

		return state.StatusCode == api.Running, nil
	})
}

// waitForExec waits until a trivial command actually runs inside the instance.
func waitForExec(ctx context.Context, client instanceServer, name string) error {
	return poll(ctx, fmt.Sprintf("instance %s to accept commands", name), func() (bool, error) {
		comm := &Communicator{Client: client, InstanceName: name}
		cmd := &packersdk.RemoteCmd{Command: "exit 0"}

		if err := comm.Start(ctx, cmd); err != nil {
			return false, nil
		}

		return cmd.Wait() == 0, nil
	})
}

// waitForNetwork waits until the instance has a usable IPv4 address.
func waitForNetwork(ctx context.Context, client instanceServer, name string) (string, error) {
	var ip string

	err := poll(ctx, fmt.Sprintf("instance %s to get an IPv4 address", name), func() (bool, error) {
		state, _, err := client.GetInstanceState(name)
		if err != nil {
			return false, nil
		}

		ip = instanceIPv4(state)

		return ip != "", nil
	})

	return ip, err
}

// poll runs check until it returns true, the context expires, or it errors.
func poll(ctx context.Context, what string, check func() (bool, error)) error {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		ok, err := check()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}

		select {
		case <-ctx.Done():
			if strings.Contains(ctx.Err().Error(), "deadline") {
				return fmt.Errorf("timed out waiting for %s; consider raising `boot_timeout`", what)
			}

			return fmt.Errorf("cancelled while waiting for %s: %w", what, ctx.Err())
		case <-ticker.C:
		}
	}
}
