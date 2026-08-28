// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

package lxd

import (
	"context"

	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/multistep/commonsteps"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

// BuilderId is unchanged from the lxc-based plugin so post-processors keying on
// it keep working.
const BuilderId = "lxd"

type Builder struct {
	config Config
	runner multistep.Runner
}

func (b *Builder) ConfigSpec() hcldec.ObjectSpec { return b.config.FlatMapstructure().HCL2Spec() }

func (b *Builder) Prepare(raws ...interface{}) ([]string, []string, error) {
	warnings, err := b.config.Prepare(raws...)
	if err != nil {
		return nil, warnings, err
	}

	generatedData := []string{"Fingerprint", "ImageAlias", "InstanceName"}

	return generatedData, warnings, nil
}

func (b *Builder) Run(ctx context.Context, ui packersdk.Ui, hook packersdk.Hook) (packersdk.Artifact, error) {
	state := new(multistep.BasicStateBag)
	state.Put("config", &b.config)
	state.Put("hook", hook)
	state.Put("ui", ui)

	steps := []multistep.Step{
		&StepConnect{},
		&StepLaunchInstance{},
		&StepWaitInstance{},
		&StepConfigureCommunicator{},
		new(commonsteps.StepProvision),
		&StepPublish{},
	}

	b.runner = commonsteps.NewRunnerWithPauseFn(steps, b.config.PackerConfig, ui, state)
	b.runner.Run(ctx, state)

	if err, ok := state.GetOk("error"); ok {
		return nil, err.(error)
	}

	fingerprint, _ := state.Get("image_fingerprint").(string)
	client, _ := state.Get("client").(instanceServer)

	artifact := &Artifact{
		Fingerprint:  fingerprint,
		InstanceName: b.config.ContainerName,
		client:       client,
		StateData: map[string]interface{}{
			"generated_data": state.Get("generated_data"),
		},
	}

	if fingerprint != "" {
		artifact.Aliases = (&StepPublish{}).aliasNames(&b.config)
	}

	return artifact, nil
}
