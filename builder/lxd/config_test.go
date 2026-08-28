// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package lxd

import (
	"strings"
	"testing"
	"time"
)

func testConfig(overrides map[string]interface{}) map[string]interface{} {
	raw := map[string]interface{}{
		"image":             "ubuntu:24.04",
		"packer_build_name": "foo",
	}

	for k, v := range overrides {
		if v == nil {
			delete(raw, k)
			continue
		}

		raw[k] = v
	}

	return raw
}

func TestConfigPrepare_Defaults(t *testing.T) {
	var c Config

	warnings, err := c.Prepare(testConfig(nil))
	if err != nil {
		t.Fatalf("Prepare: %s", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}

	if c.ContainerName != "packer-foo" {
		t.Errorf("ContainerName = %q, want packer-foo", c.ContainerName)
	}
	if c.OutputImage != "packer-foo" {
		t.Errorf("OutputImage = %q, want packer-foo", c.OutputImage)
	}
	if len(c.Profiles) != 1 || c.Profiles[0] != "default" {
		t.Errorf("Profiles = %v, want [default]", c.Profiles)
	}
	if c.InstanceType != InstanceTypeContainer {
		t.Errorf("InstanceType = %q, want %q", c.InstanceType, InstanceTypeContainer)
	}
	if c.BootTimeout != 2*time.Minute {
		t.Errorf("BootTimeout = %s, want 2m", c.BootTimeout)
	}
}

func TestConfigPrepare_ImageRequired(t *testing.T) {
	var c Config

	if _, err := c.Prepare(testConfig(map[string]interface{}{"image": nil})); err == nil {
		t.Fatal("expected an error when image is unset")
	}
}

func TestConfigPrepare_OutputImageDefaultsToContainerName(t *testing.T) {
	var c Config

	if _, err := c.Prepare(testConfig(map[string]interface{}{"container_name": "my-box"})); err != nil {
		t.Fatalf("Prepare: %s", err)
	}

	if c.OutputImage != "my-box" {
		t.Errorf("OutputImage = %q, want my-box", c.OutputImage)
	}
}

func TestConfigPrepare_ProfileAndProfiles(t *testing.T) {
	tests := []struct {
		name    string
		raw     map[string]interface{}
		want    []string
		wantErr bool
	}{
		{name: "neither", raw: nil, want: []string{"default"}},
		{name: "profile", raw: map[string]interface{}{"profile": "web"}, want: []string{"web"}},
		{name: "profiles", raw: map[string]interface{}{"profiles": []string{"a", "b"}}, want: []string{"a", "b"}},
		{
			name:    "both",
			raw:     map[string]interface{}{"profile": "web", "profiles": []string{"a"}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c Config

			_, err := c.Prepare(testConfig(tt.raw))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Prepare: %s", err)
			}

			if strings.Join(c.Profiles, ",") != strings.Join(tt.want, ",") {
				t.Errorf("Profiles = %v, want %v", c.Profiles, tt.want)
			}
		})
	}
}

func TestConfigPrepare_InstanceType(t *testing.T) {
	tests := []struct {
		name    string
		raw     map[string]interface{}
		want    string
		wantErr bool
	}{
		{name: "default", raw: nil, want: InstanceTypeContainer},
		{name: "virtual_machine", raw: map[string]interface{}{"virtual_machine": true}, want: InstanceTypeVirtualMachine},
		{name: "instance_type", raw: map[string]interface{}{"instance_type": "virtual-machine"}, want: InstanceTypeVirtualMachine},
		{
			name:    "both",
			raw:     map[string]interface{}{"virtual_machine": true, "instance_type": "container"},
			wantErr: true,
		},
		{name: "invalid", raw: map[string]interface{}{"instance_type": "toaster"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c Config

			_, err := c.Prepare(testConfig(tt.raw))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Prepare: %s", err)
			}

			if c.InstanceType != tt.want {
				t.Errorf("InstanceType = %q, want %q", c.InstanceType, tt.want)
			}
		})
	}
}

// Templates written for the lxc-based plugin must still load; the options that
// no longer apply are accepted, ignored, and warned about.
func TestConfigPrepare_DeprecatedOptionsWarnButDoNotFail(t *testing.T) {
	for _, option := range []string{"command_wrapper", "init_sleep"} {
		t.Run(option, func(t *testing.T) {
			var c Config

			warnings, err := c.Prepare(testConfig(map[string]interface{}{option: "3"}))
			if err != nil {
				t.Fatalf("Prepare: %s", err)
			}

			if len(warnings) != 1 || !strings.Contains(warnings[0], option) {
				t.Fatalf("warnings = %v, want one mentioning %s", warnings, option)
			}
		})
	}
}

func TestConfigPrepare_BootTimeout(t *testing.T) {
	var c Config

	if _, err := c.Prepare(testConfig(map[string]interface{}{"boot_timeout": "45s"})); err != nil {
		t.Fatalf("Prepare: %s", err)
	}

	if c.BootTimeout != 45*time.Second {
		t.Errorf("BootTimeout = %s, want 45s", c.BootTimeout)
	}
}

// The old plugin parsed init_sleep mid-build, so a bad value failed the build
// rather than validation. boot_timeout is checked up front.
func TestConfigPrepare_BootTimeoutInvalid(t *testing.T) {
	var c Config

	if _, err := c.Prepare(testConfig(map[string]interface{}{"boot_timeout": "not-a-duration"})); err == nil {
		t.Fatal("expected an error for an unparseable boot_timeout")
	}
}

func TestConfigPrepare_MutuallyExclusiveConnection(t *testing.T) {
	var c Config

	_, err := c.Prepare(testConfig(map[string]interface{}{
		"remote_name":    "prod",
		"daemon_address": "https://example:8443",
	}))
	if err == nil {
		t.Fatal("expected an error when both remote_name and daemon_address are set")
	}
}

func TestConfigPrepare_ClientCertRequiresKey(t *testing.T) {
	var c Config

	if _, err := c.Prepare(testConfig(map[string]interface{}{"client_cert": "/tmp/c.pem"})); err == nil {
		t.Fatal("expected an error when client_cert is set without client_key")
	}
}

// An ephemeral instance is destroyed on stop, so it can never be published.
func TestConfigPrepare_EphemeralConflictsWithPublish(t *testing.T) {
	var c Config

	if _, err := c.Prepare(testConfig(map[string]interface{}{"ephemeral": true})); err == nil {
		t.Fatal("expected an error for an ephemeral instance without skip_publish")
	}

	var ok Config
	if _, err := ok.Prepare(testConfig(map[string]interface{}{
		"ephemeral":    true,
		"skip_publish": true,
	})); err != nil {
		t.Fatalf("ephemeral with skip_publish should be valid: %s", err)
	}
}

func TestConfigPrepare_ImageRemoteProtocol(t *testing.T) {
	var c Config

	if _, err := c.Prepare(testConfig(map[string]interface{}{
		"image_remote_url": "https://images.example.com",
	})); err != nil {
		t.Fatalf("Prepare: %s", err)
	}
	if c.ImageRemoteProtocol != "simplestreams" {
		t.Errorf("ImageRemoteProtocol = %q, want simplestreams", c.ImageRemoteProtocol)
	}

	var bad Config
	if _, err := bad.Prepare(testConfig(map[string]interface{}{
		"image_remote_protocol": "carrier-pigeon",
	})); err == nil {
		t.Fatal("expected an error for an unknown image_remote_protocol")
	}
}
