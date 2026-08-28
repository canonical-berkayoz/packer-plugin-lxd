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

// StepPublish stops the build instance and turns it into an image.
type StepPublish struct{}

func (s *StepPublish) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	config := state.Get("config").(*Config)
	ui := state.Get("ui").(packersdk.Ui)
	client := state.Get("client").(instanceServer)

	if config.SkipPublish {
		ui.Say("skip_publish is true. Skipping publish step.")
		return multistep.ActionContinue
	}

	ui.Say(fmt.Sprintf("Stopping instance %s...", config.ContainerName))

	op, err := client.UpdateInstanceState(config.ContainerName, api.InstanceStatePut{
		Action:  "stop",
		Timeout: -1,
		Force:   true,
	}, "")
	if err == nil {
		err = op.Wait()
	}
	if err != nil {
		return halt(state, ui, fmt.Errorf("stopping instance %s: %w", config.ContainerName, err))
	}

	aliases := s.aliases(config)

	// Creating an alias that already exists is an error, so clear conflicts
	// first when the user asked us to.
	if config.ReuseAlias {
		for _, alias := range aliases {
			if _, _, err := client.GetImageAlias(alias.Name); err != nil {
				continue
			}

			ui.Say(fmt.Sprintf("Removing existing alias %s...", alias.Name))

			if err := client.DeleteImageAlias(alias.Name); err != nil {
				return halt(state, ui, fmt.Errorf("removing existing alias %s: %w", alias.Name, err))
			}
		}
	}

	ui.Say("Publishing instance as an image...")

	req := api.ImagesPost{
		Source: &api.ImagesPostSource{
			Type: "instance",
			Name: config.ContainerName,
		},
		CompressionAlgorithm: config.CompressionAlgorithm,
		Aliases:              aliases,
	}
	req.Public = config.PublishPublic
	// Leave Properties nil when there are none: an empty map would clear the
	// properties inherited from the source image.
	if len(config.PublishProperties) > 0 {
		req.Properties = config.PublishProperties
	}

	publishOp, err := client.CreateImage(req, nil)
	if err == nil {
		err = publishOp.Wait()
	}
	if err != nil {
		return halt(state, ui, fmt.Errorf("publishing instance %s: %w", config.ContainerName, err))
	}

	// The fingerprint comes back as structured operation metadata. The
	// lxc-based plugin scraped it out of human-readable stdout with a regex.
	fingerprint, ok := publishOp.Get().Metadata["fingerprint"].(string)
	if !ok || fingerprint == "" {
		return halt(state, ui, fmt.Errorf("publish operation returned no image fingerprint"))
	}

	ui.Say(fmt.Sprintf("Created image: %s", fingerprint))

	state.Put("image_fingerprint", fingerprint)

	if config.PublishRemoteName != "" {
		if err := s.copyToRemote(config, ui, client, fingerprint, aliases); err != nil {
			return halt(state, ui, err)
		}
	}

	return multistep.ActionContinue
}

func (s *StepPublish) Cleanup(state multistep.StateBag) {}

// aliases returns the image aliases to create, with output_image first.
func (s *StepPublish) aliases(config *Config) []api.ImageAlias {
	seen := map[string]bool{}
	aliases := []api.ImageAlias{}

	for _, name := range append([]string{config.OutputImage}, config.PublishAliases...) {
		if name == "" || seen[name] {
			continue
		}

		seen[name] = true
		aliases = append(aliases, api.ImageAlias{Name: name})
	}

	return aliases
}

// copyToRemote copies the freshly published image to another LXD remote.
func (s *StepPublish) copyToRemote(
	config *Config,
	ui packersdk.Ui,
	client instanceServer,
	fingerprint string,
	aliases []api.ImageAlias,
) error {
	ui.Say(fmt.Sprintf("Copying image to remote %s...", config.PublishRemoteName))

	source, ok := client.(lxdclient.InstanceServer)
	if !ok {
		return fmt.Errorf("copying to remote %s requires a real LXD connection", config.PublishRemoteName)
	}

	cfg := loadLXDConfig()

	dest, err := cfg.GetInstanceServer(config.PublishRemoteName)
	if err != nil {
		return fmt.Errorf("connecting to publish remote %q: %w", config.PublishRemoteName, err)
	}

	image, _, err := source.GetImage(fingerprint)
	if err != nil {
		return fmt.Errorf("reading published image %s: %w", fingerprint, err)
	}

	op, err := dest.CopyImage(source, *image, &lxdclient.ImageCopyArgs{
		Aliases: aliases,
		Public:  config.PublishPublic,
	})
	if err == nil {
		err = op.Wait()
	}
	if err != nil {
		return fmt.Errorf("copying image to remote %q: %w", config.PublishRemoteName, err)
	}

	return nil
}

// aliasNames returns the names of the aliases created for the published image.
func (s *StepPublish) aliasNames(config *Config) []string {
	aliases := s.aliases(config)

	names := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		names = append(names, alias.Name)
	}

	return names
}
