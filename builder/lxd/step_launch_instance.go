// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package lxd

import (
	"context"
	"fmt"

	lxdclient "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

// StepLaunchInstance creates and starts the instance the build runs in.
type StepLaunchInstance struct {
	launched bool
}

func (s *StepLaunchInstance) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	config := state.Get("config").(*Config)
	ui := state.Get("ui").(packersdk.Ui)
	client := state.Get("client").(instanceServer)

	ui.Say(fmt.Sprintf("Resolving source image %s...", config.Image))

	// sourceImage needs a real ImageServer to hand to CreateInstanceFromImage;
	// the same connection satisfies both views.
	local, _ := state.Get("client").(lxdclient.ImageServer)

	imageServer, image, err := config.sourceImage(local)
	if err != nil {
		return halt(state, ui, err)
	}

	ui.Say(fmt.Sprintf("Launching instance %s from image %s...", config.ContainerName, image.Fingerprint))

	req := api.InstancesPost{
		Name:  config.ContainerName,
		Type:  api.InstanceType(config.InstanceType),
		Start: true,
	}
	req.Config = config.LaunchConfig
	req.Devices = config.LaunchDevices
	req.Profiles = config.Profiles
	req.Ephemeral = config.Ephemeral

	op, err := client.CreateInstanceFromImage(imageServer, *image, req)
	if err != nil {
		return halt(state, ui, fmt.Errorf("creating instance %s: %w", config.ContainerName, err))
	}

	if err := op.Wait(); err != nil {
		return halt(state, ui, fmt.Errorf("creating instance %s: %w", config.ContainerName, err))
	}

	s.launched = true
	state.Put("instance_name", config.ContainerName)

	return multistep.ActionContinue
}

func (s *StepLaunchInstance) Cleanup(state multistep.StateBag) {
	if !s.launched {
		return
	}

	config := state.Get("config").(*Config)
	ui := state.Get("ui").(packersdk.Ui)
	client := state.Get("client").(instanceServer)

	// With skip_publish the instance is the point of the build; leave it up so
	// it can be inspected.
	if config.SkipPublish {
		ui.Say(fmt.Sprintf("skip_publish is set; leaving instance %s running.", config.ContainerName))
		return
	}

	ui.Say(fmt.Sprintf("Deleting instance %s...", config.ContainerName))

	// Stop first: a running instance cannot be deleted, and force covers the
	// case where it is still running after a failed step.
	if op, err := client.UpdateInstanceState(config.ContainerName, api.InstanceStatePut{
		Action:  "stop",
		Timeout: -1,
		Force:   true,
	}, ""); err == nil {
		_ = op.Wait()
	}

	op, err := client.DeleteInstance(config.ContainerName, true)
	if err == nil {
		err = op.Wait()
	}
	if err != nil {
		ui.Error(fmt.Sprintf("Error deleting instance %s: %s", config.ContainerName, err))
	}
}
