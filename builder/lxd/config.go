// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type Config

package lxd

import (
	"fmt"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/common"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
)

// Instance types accepted by the `instance_type` option.
const (
	InstanceTypeContainer      = "container"
	InstanceTypeVirtualMachine = "virtual-machine"
)

type Config struct {
	common.PackerConfig `mapstructure:",squash"`

	// The source image to use when creating the build instance. This can be a
	// local or remote image, given as an alias or a fingerprint. Remotes are
	// resolved the same way the `lxc` CLI resolves them, so both the built-in
	// remotes (`images:`, `ubuntu:`, `ubuntu-daily:`, ...) and any remote
	// configured in your LXD client config may be used.
	// E.g. `my-base-image`, `ubuntu:24.04`, `images:alpine/edge`, `08fababf6f27`.
	Image string `mapstructure:"image" required:"true"`
	// Explicit image server URL, bypassing remote-name resolution. When set,
	// `image` is treated as an alias or fingerprint on this server.
	ImageRemoteURL string `mapstructure:"image_remote_url" required:"false"`
	// Protocol of `image_remote_url`; either `simplestreams` or `lxd`.
	// Defaults to `simplestreams`.
	ImageRemoteProtocol string `mapstructure:"image_remote_protocol" required:"false"`

	// The name of the build instance. Defaults to `packer-{{PackerBuildName}}`.
	ContainerName string `mapstructure:"container_name" required:"false"`
	// The name of the output artifact. Defaults to `container_name`.
	OutputImage string `mapstructure:"output_image" required:"false"`
	// The (optional) name of the LXD remote on which to publish the image.
	PublishRemoteName string `mapstructure:"publish_remote_name" required:"false"`

	// The name of a remote from your LXD client config to build on. Mutually
	// exclusive with `daemon_address`. Defaults to the client config's default
	// remote, or the local unix socket.
	RemoteName string `mapstructure:"remote_name" required:"false"`
	// Address of the LXD daemon to build on: either a unix socket path or an
	// `https://host:8443` URL. Mutually exclusive with `remote_name`.
	DaemonAddress string `mapstructure:"daemon_address" required:"false"`
	// Path to a PEM client certificate used to authenticate to the daemon.
	ClientCertFile string `mapstructure:"client_cert" required:"false"`
	// Path to the PEM client key matching `client_cert`.
	ClientKeyFile string `mapstructure:"client_key" required:"false"`
	// Path to the daemon's PEM server certificate, for pinning.
	ServerCertFile string `mapstructure:"server_cert" required:"false"`
	// Do not verify the daemon's TLS certificate. Defaults to false.
	InsecureSkipVerify bool `mapstructure:"insecure_skip_verify" required:"false"`
	// The LXD project to build in. Defaults to the remote's project.
	Project string `mapstructure:"project" required:"false"`
	// The cluster member to target when building against an LXD cluster.
	Target string `mapstructure:"target" required:"false"`

	// A single profile to apply to the build instance. Defaults to `default`.
	// Use `profiles` to apply more than one.
	Profile string `mapstructure:"profile" required:"false"`
	// The list of profiles to apply to the build instance. Mutually exclusive
	// with `profile`.
	Profiles []string `mapstructure:"profiles" required:"false"`
	// Key/value pairs set as instance configuration keys on the build instance,
	// equivalent to `lxc launch --config`. Defaults to empty.
	LaunchConfig map[string]string `mapstructure:"launch_config" required:"false"`
	// Devices to attach to the build instance, keyed by device name.
	LaunchDevices map[string]map[string]string `mapstructure:"launch_devices" required:"false"`
	// Create a virtual-machine image instead of a container image; defaults to
	// false. Equivalent to setting `instance_type` to `virtual-machine`.
	VirtualMachine bool `mapstructure:"virtual_machine" required:"false"`
	// The type of instance to build: `container` or `virtual-machine`.
	// Defaults to `container`. Mutually exclusive with `virtual_machine`.
	InstanceType string `mapstructure:"instance_type" required:"false"`
	// Make the build instance ephemeral. Defaults to false; an ephemeral
	// instance cannot be stopped and published.
	Ephemeral bool `mapstructure:"ephemeral" required:"false"`

	// How long to wait for the instance to become ready to run commands.
	// Defaults to `2m`.
	BootTimeout time.Duration `mapstructure:"boot_timeout" required:"false"`
	// Additionally wait for the instance to be assigned a non-loopback IPv4
	// address before provisioning. Defaults to false.
	WaitForNetwork bool `mapstructure:"wait_for_network" required:"false"`

	// Key/value pairs set as properties on the published image. Most commonly
	// used to set `description`.
	PublishProperties map[string]string `mapstructure:"publish_properties" required:"false"`
	// Additional aliases to create for the published image, alongside
	// `output_image`.
	PublishAliases []string `mapstructure:"publish_aliases" required:"false"`
	// Mark the published image public. Defaults to false.
	PublishPublic bool `mapstructure:"publish_public" required:"false"`
	// Compression algorithm to use when publishing, e.g. `gzip`, `xz`, or
	// `none`. Defaults to the server's setting.
	CompressionAlgorithm string `mapstructure:"compression_algorithm" required:"false"`
	// Delete any pre-existing alias that conflicts with the aliases being
	// created, instead of failing. Defaults to false.
	ReuseAlias bool `mapstructure:"reuse_alias" required:"false"`
	// Skip publishing the image. The build instance is left running so it can
	// be inspected. Defaults to false.
	SkipPublish bool `mapstructure:"skip_publish" required:"false"`

	// Environment variables set for every command run by provisioners.
	ExecEnvironment map[string]string `mapstructure:"exec_environment" required:"false"`
	// The uid to run provisioner commands as. Defaults to 0 (root).
	ExecUser uint32 `mapstructure:"exec_user" required:"false"`
	// The gid to run provisioner commands as. Defaults to 0 (root).
	ExecGroup uint32 `mapstructure:"exec_group" required:"false"`
	// The working directory for provisioner commands. Defaults to the image's
	// configured working directory.
	ExecCwd string `mapstructure:"exec_cwd" required:"false"`

	// Deprecated and ignored. The plugin talks to the LXD API directly, so
	// there is no shell command to wrap.
	CommandWrapper string `mapstructure:"command_wrapper" required:"false"`
	// Deprecated and ignored. Superseded by a real readiness probe; see
	// `boot_timeout`.
	InitSleep string `mapstructure:"init_sleep" required:"false"`

	ctx interpolate.Context
}

func (c *Config) Prepare(raws ...interface{}) ([]string, error) {
	err := config.Decode(c, &config.DecodeOpts{
		PluginType:         "packer.builder.lxd",
		Interpolate:        true,
		InterpolateContext: &c.ctx,
	}, raws...)
	if err != nil {
		return nil, err
	}

	var warnings []string
	var errs *packersdk.MultiError

	if c.Image == "" {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf(
			"`image` is a required parameter for LXD. Please specify an image by alias or fingerprint. e.g. `ubuntu:24.04`"))
	}

	if c.ContainerName == "" {
		c.ContainerName = fmt.Sprintf("packer-%s", c.PackerBuildName)
	}

	if c.OutputImage == "" {
		c.OutputImage = c.ContainerName
	}

	// `profile` and `profiles` express the same thing; accept either but not
	// both, and normalise onto Profiles.
	switch {
	case c.Profile != "" && len(c.Profiles) > 0:
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf(
			"only one of `profile` and `profiles` may be set"))
	case c.Profile != "":
		c.Profiles = []string{c.Profile}
	case len(c.Profiles) == 0:
		c.Profiles = []string{"default"}
	}

	// Likewise for `virtual_machine` and `instance_type`.
	switch {
	case c.VirtualMachine && c.InstanceType != "":
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf(
			"only one of `virtual_machine` and `instance_type` may be set"))
	case c.VirtualMachine:
		c.InstanceType = InstanceTypeVirtualMachine
	case c.InstanceType == "":
		c.InstanceType = InstanceTypeContainer
	case c.InstanceType != InstanceTypeContainer && c.InstanceType != InstanceTypeVirtualMachine:
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf(
			"`instance_type` must be %q or %q, got %q",
			InstanceTypeContainer, InstanceTypeVirtualMachine, c.InstanceType))
	}

	if c.RemoteName != "" && c.DaemonAddress != "" {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf(
			"only one of `remote_name` and `daemon_address` may be set"))
	}

	if (c.ClientCertFile == "") != (c.ClientKeyFile == "") {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf(
			"`client_cert` and `client_key` must be set together"))
	}

	switch c.ImageRemoteProtocol {
	case "":
		if c.ImageRemoteURL != "" {
			c.ImageRemoteProtocol = "simplestreams"
		}
	case "simplestreams", "lxd":
	default:
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf(
			"`image_remote_protocol` must be \"simplestreams\" or \"lxd\", got %q", c.ImageRemoteProtocol))
	}

	if c.BootTimeout == 0 {
		c.BootTimeout = 2 * time.Minute
	}
	if c.BootTimeout < 0 {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf(
			"`boot_timeout` must not be negative, got %s", c.BootTimeout))
	}

	if c.Ephemeral && !c.SkipPublish {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf(
			"`ephemeral` instances are destroyed on stop and cannot be published; "+
				"set `skip_publish` or drop `ephemeral`"))
	}

	// Options that only made sense while shelling out to `lxc`.
	if c.CommandWrapper != "" {
		warnings = append(warnings, "`command_wrapper` is deprecated and ignored: "+
			"this plugin talks to the LXD API directly and never invokes a shell. "+
			"Use `remote_name` or `daemon_address` to build against a remote daemon.")
		c.CommandWrapper = ""
	}
	if c.InitSleep != "" {
		warnings = append(warnings, "`init_sleep` is deprecated and ignored: "+
			"the plugin now waits for the instance to actually become ready. "+
			"Use `boot_timeout` to bound that wait.")
		c.InitSleep = ""
	}

	if errs != nil && len(errs.Errors) > 0 {
		return warnings, errs
	}

	return warnings, nil
}
