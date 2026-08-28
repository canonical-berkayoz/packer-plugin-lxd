// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package lxd

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	lxdclient "github.com/canonical/lxd/client"
	lxdconfig "github.com/canonical/lxd/lxc/config"
	"github.com/canonical/lxd/shared/api"
	"github.com/pkg/sftp"
)

// instanceServer is the subset of lxdclient.InstanceServer this builder uses.
// Narrowing it keeps the steps and the communicator unit-testable against a
// fake, with no LXD daemon in the loop.
type instanceServer interface {
	GetServer() (*api.Server, string, error)
	HasExtension(extension string) bool

	CreateInstanceFromImage(source lxdclient.ImageServer, image api.Image, req api.InstancesPost) (lxdclient.RemoteOperation, error)
	GetInstanceState(name string) (*api.InstanceState, string, error)
	UpdateInstanceState(name string, state api.InstanceStatePut, ETag string) (lxdclient.Operation, error)
	DeleteInstance(name string, force bool) (lxdclient.Operation, error)

	ExecInstance(name string, exec api.InstanceExecPost, args *lxdclient.InstanceExecArgs) (lxdclient.Operation, error)
	GetInstanceFileSFTP(name string) (*sftp.Client, error)

	CreateImage(image api.ImagesPost, args *lxdclient.ImageCreateArgs) (lxdclient.Operation, error)
	DeleteImage(fingerprint string) (lxdclient.Operation, error)
	CreateImageAlias(alias api.ImageAliasesPost) error
	DeleteImageAlias(name string) error
	GetImageAlias(name string) (*api.ImageAliasesEntry, string, error)
}

// lxdclient.InstanceServer must satisfy our narrowed view of it.
var _ instanceServer = (lxdclient.InstanceServer)(nil)

// lxdConfigDir mirrors how the lxc CLI locates its client configuration, so
// remotes the user has already configured resolve exactly as they do for lxc.
func lxdConfigDir() string {
	if dir := os.Getenv("LXD_CONF"); dir != "" {
		return dir
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// The snap keeps its client config in its own tree.
	snapDir := filepath.Join(home, "snap", "lxd", "common", "config")
	if _, err := os.Stat(filepath.Join(snapDir, "config.yml")); err == nil {
		return snapDir
	}

	return filepath.Join(home, ".config", "lxc")
}

// loadLXDConfig returns the user's lxc client config, falling back to the
// built-in remotes (local, images, ubuntu, ubuntu-daily, ...) when the user has
// no config file of their own.
func loadLXDConfig() *lxdconfig.Config {
	dir := lxdConfigDir()
	if dir != "" {
		if cfg, err := lxdconfig.LoadConfig(filepath.Join(dir, "config.yml")); err == nil {
			return cfg
		}
	}

	cfg := lxdconfig.DefaultConfig()
	cfg.ConfigDir = dir
	return cfg
}

// connectionArgs builds the TLS/connection options common to every connection
// this builder makes.
func (c *Config) connectionArgs() (*lxdclient.ConnectionArgs, error) {
	args := &lxdclient.ConnectionArgs{
		UserAgent:          "packer-plugin-lxd",
		InsecureSkipVerify: c.InsecureSkipVerify,
	}

	for _, f := range []struct {
		path string
		dst  *string
		what string
	}{
		{c.ClientCertFile, &args.TLSClientCert, "client_cert"},
		{c.ClientKeyFile, &args.TLSClientKey, "client_key"},
		{c.ServerCertFile, &args.TLSServerCert, "server_cert"},
	} {
		if f.path == "" {
			continue
		}

		content, err := os.ReadFile(f.path)
		if err != nil {
			return nil, fmt.Errorf("reading `%s` from %s: %w", f.what, f.path, err)
		}

		*f.dst = string(content)
	}

	return args, nil
}

// connectDaemon connects to the LXD daemon this build should run against and
// scopes the returned client to the configured project and cluster member.
func (c *Config) connectDaemon() (lxdclient.InstanceServer, error) {
	args, err := c.connectionArgs()
	if err != nil {
		return nil, err
	}

	var server lxdclient.InstanceServer

	switch {
	case c.DaemonAddress != "":
		server, err = connectAddress(c.DaemonAddress, args)
	default:
		cfg := loadLXDConfig()

		remote := c.RemoteName
		if remote == "" {
			remote = cfg.DefaultRemote
		}
		if remote == "" {
			remote = "local"
		}

		server, err = cfg.GetInstanceServerWithConnectionArgs(remote, args)
		if err != nil {
			err = fmt.Errorf("connecting to LXD remote %q: %w", remote, err)
		}
	}

	if err != nil {
		return nil, err
	}

	if c.Project != "" {
		server = server.UseProject(c.Project)
	}
	if c.Target != "" {
		server = server.UseTarget(c.Target)
	}

	return server, nil
}

// connectAddress connects to an explicitly configured daemon address, which may
// be either an https URL or the path to a unix socket.
func connectAddress(address string, args *lxdclient.ConnectionArgs) (lxdclient.InstanceServer, error) {
	if strings.HasPrefix(address, "https://") || strings.HasPrefix(address, "http://") {
		server, err := lxdclient.ConnectLXD(address, args)
		if err != nil {
			return nil, fmt.Errorf("connecting to LXD daemon at %s: %w", address, err)
		}

		return server, nil
	}

	// Anything else is treated as a unix socket path. An empty path lets the
	// client apply its own search order ($LXD_SOCKET, $LXD_DIR, the snap path).
	server, err := lxdclient.ConnectLXDUnix(address, args)
	if err != nil {
		return nil, fmt.Errorf("connecting to LXD daemon on unix socket %q: %w", address, err)
	}

	return server, nil
}

// sourceImage resolves the configured `image` to a concrete image on some image
// server, ready to be handed to CreateInstanceFromImage.
//
// This is the piece the old plugin got for free by shelling out to `lxc`: the
// CLI resolved `ubuntu:24.04` against its own remote table. We reuse LXD's own
// config package so both the built-in remotes and the user's configured ones
// keep working.
func (c *Config) sourceImage(local lxdclient.ImageServer) (lxdclient.ImageServer, *api.Image, error) {
	var (
		server lxdclient.ImageServer
		name   string
		err    error
	)

	if c.ImageRemoteURL != "" {
		args, argsErr := c.connectionArgs()
		if argsErr != nil {
			return nil, nil, argsErr
		}

		name = c.Image

		if c.ImageRemoteProtocol == "lxd" {
			server, err = lxdclient.ConnectPublicLXD(c.ImageRemoteURL, args)
		} else {
			server, err = lxdclient.ConnectSimpleStreams(c.ImageRemoteURL, args)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("connecting to image server %s: %w", c.ImageRemoteURL, err)
		}
	} else {
		cfg := loadLXDConfig()

		var remote string
		remote, name, err = cfg.ParseRemote(c.Image)
		if err != nil {
			return nil, nil, fmt.Errorf("parsing image %q: %w", c.Image, err)
		}

		// An image on the build daemon itself: reuse the connection we have,
		// which also keeps any project scoping applied.
		if remote == "" || remote == cfg.DefaultRemote || remote == "local" {
			server = local
		} else {
			server, err = cfg.GetImageServer(remote)
			if err != nil {
				return nil, nil, fmt.Errorf("connecting to image remote %q: %w", remote, err)
			}
		}
	}

	if name == "" {
		return nil, nil, fmt.Errorf("no image name or fingerprint given in %q", c.Image)
	}

	image, err := resolveImage(server, name, c.InstanceType)
	if err != nil {
		return nil, nil, err
	}

	return server, image, nil
}

// resolveImage turns an alias or a fingerprint into a concrete image, trying the
// alias first because that is overwhelmingly the common case.
func resolveImage(server lxdclient.ImageServer, name string, instanceType string) (*api.Image, error) {
	fingerprint := name

	alias, _, err := server.GetImageAliasType(instanceType, name)
	if err == nil && alias != nil {
		fingerprint = alias.Target
	}

	image, _, err := server.GetImage(fingerprint)
	if err != nil {
		return nil, fmt.Errorf("looking up image %q: %w", name, err)
	}

	return image, nil
}

// instanceIPv4 returns the first non-loopback IPv4 address assigned to the
// instance, or "" if it has none yet.
func instanceIPv4(state *api.InstanceState) string {
	if state == nil {
		return ""
	}

	for name, network := range state.Network {
		if name == "lo" {
			continue
		}

		for _, addr := range network.Addresses {
			if addr.Family != "inet" {
				continue
			}

			ip := net.ParseIP(addr.Address)
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}

			return addr.Address
		}
	}

	return ""
}
