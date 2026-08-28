// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package lxd

import (
	"os"
	"path/filepath"
	"testing"

	lxdconfig "github.com/canonical/lxd/lxc/config"
	"github.com/canonical/lxd/shared/api"
)

// The remotes the lxc CLI ships with must resolve without any user config, so
// templates using `ubuntu:` or `images:` keep working.
func TestLoadLXDConfig_HasBuiltinRemotes(t *testing.T) {
	t.Setenv("LXD_CONF", t.TempDir())

	cfg := loadLXDConfig()

	for _, remote := range []string{"local", "images", "ubuntu", "ubuntu-daily"} {
		if _, ok := cfg.Remotes[remote]; !ok {
			t.Errorf("built-in remote %q is missing", remote)
		}
	}
}

func TestLoadLXDConfig_ReadsUserRemotes(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LXD_CONF", dir)

	err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(
		"default-remote: myremote\n"+
			"remotes:\n"+
			"  myremote:\n"+
			"    addr: https://lxd.example.com:8443\n"+
			"    protocol: lxd\n"), 0600)
	if err != nil {
		t.Fatalf("writing config: %s", err)
	}

	cfg := loadLXDConfig()

	remote, ok := cfg.Remotes["myremote"]
	if !ok {
		t.Fatal("user-configured remote myremote was not loaded")
	}
	if remote.Addr != "https://lxd.example.com:8443" {
		t.Errorf("addr = %q, want https://lxd.example.com:8443", remote.Addr)
	}
	if cfg.DefaultRemote != "myremote" {
		t.Errorf("DefaultRemote = %q, want myremote", cfg.DefaultRemote)
	}
}

// `image` accepts the same remote:name forms the lxc CLI accepts.
func TestParseRemote_ImageForms(t *testing.T) {
	t.Setenv("LXD_CONF", t.TempDir())
	cfg := loadLXDConfig()

	tests := []struct {
		image      string
		wantRemote string
		wantName   string
	}{
		{"ubuntu:24.04", "ubuntu", "24.04"},
		{"images:alpine/edge", "images", "alpine/edge"},
		{"ubuntu-daily:noble", "ubuntu-daily", "noble"},
		{"my-base-image", "local", "my-base-image"},
		{"08fababf6f27", "local", "08fababf6f27"},
	}

	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			remote, name, err := cfg.ParseRemote(tt.image)
			if err != nil {
				t.Fatalf("ParseRemote(%q): %s", tt.image, err)
			}

			if remote != tt.wantRemote {
				t.Errorf("remote = %q, want %q", remote, tt.wantRemote)
			}
			if name != tt.wantName {
				t.Errorf("name = %q, want %q", name, tt.wantName)
			}
		})
	}
}

func TestParseRemote_UnknownRemoteIsAnError(t *testing.T) {
	t.Setenv("LXD_CONF", t.TempDir())
	cfg := loadLXDConfig()

	if _, _, err := cfg.ParseRemote("nosuchremote:foo"); err == nil {
		t.Fatal("expected an error for an unknown remote")
	}
}

func TestLXDConfigDir_PrefersEnvVar(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LXD_CONF", dir)

	if got := lxdConfigDir(); got != dir {
		t.Errorf("lxdConfigDir() = %q, want %q", got, dir)
	}
}

func TestConnectionArgs_LoadsCertificates(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "client.crt")
	keyPath := filepath.Join(dir, "client.key")

	if err := os.WriteFile(certPath, []byte("CERT"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte("KEY"), 0600); err != nil {
		t.Fatal(err)
	}

	c := &Config{ClientCertFile: certPath, ClientKeyFile: keyPath, InsecureSkipVerify: true}

	args, err := c.connectionArgs()
	if err != nil {
		t.Fatalf("connectionArgs: %s", err)
	}

	if args.TLSClientCert != "CERT" || args.TLSClientKey != "KEY" {
		t.Errorf("cert/key = %q/%q, want CERT/KEY", args.TLSClientCert, args.TLSClientKey)
	}
	if !args.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be propagated")
	}
}

func TestConnectionArgs_MissingCertFileIsAnError(t *testing.T) {
	c := &Config{
		ClientCertFile: filepath.Join(t.TempDir(), "absent.crt"),
		ClientKeyFile:  filepath.Join(t.TempDir(), "absent.key"),
	}

	if _, err := c.connectionArgs(); err == nil {
		t.Fatal("expected an error for an unreadable client_cert")
	}
}

func TestInstanceIPv4(t *testing.T) {
	tests := []struct {
		name  string
		state *api.InstanceState
		want  string
	}{
		{name: "nil", state: nil, want: ""},
		{
			name: "loopback only",
			state: &api.InstanceState{Network: map[string]api.InstanceStateNetwork{
				"lo": {Addresses: []api.InstanceStateNetworkAddress{
					{Family: "inet", Address: "127.0.0.1"},
				}},
			}},
			want: "",
		},
		{
			name: "skips ipv6 and link-local",
			state: &api.InstanceState{Network: map[string]api.InstanceStateNetwork{
				"eth0": {Addresses: []api.InstanceStateNetworkAddress{
					{Family: "inet6", Address: "fe80::1"},
					{Family: "inet", Address: "169.254.1.1"},
					{Family: "inet", Address: "10.0.0.5"},
				}},
			}},
			want: "10.0.0.5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := instanceIPv4(tt.state); got != tt.want {
				t.Errorf("instanceIPv4() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Guard the assumption the whole client layer rests on.
func TestDefaultConfigRemotesAreLXDConfigType(t *testing.T) {
	var _ *lxdconfig.Config = lxdconfig.DefaultConfig()
}
