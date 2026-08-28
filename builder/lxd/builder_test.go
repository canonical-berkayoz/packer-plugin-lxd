// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package lxd

import (
	"testing"

	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

func TestBuilder_ImplementsBuilder(t *testing.T) {
	var raw interface{} = new(Builder)
	if _, ok := raw.(packersdk.Builder); !ok {
		t.Fatal("Builder must implement packersdk.Builder")
	}
}

func TestBuilder_ConfigSpec(t *testing.T) {
	spec := new(Builder).ConfigSpec()

	// Every option the lxc-based plugin accepted must still be addressable so
	// existing templates keep parsing.
	for _, key := range []string{
		"image", "container_name", "output_image", "publish_remote_name",
		"command_wrapper", "profile", "init_sleep", "publish_properties",
		"launch_config", "virtual_machine", "skip_publish",
	} {
		if _, ok := spec[key]; !ok {
			t.Errorf("config spec is missing the legacy option %q", key)
		}
	}

	// ...alongside the options the API-based builder adds.
	for _, key := range []string{
		"remote_name", "daemon_address", "project", "target", "profiles",
		"instance_type", "boot_timeout", "publish_aliases", "reuse_alias",
	} {
		if _, ok := spec[key]; !ok {
			t.Errorf("config spec is missing the new option %q", key)
		}
	}
}

func TestBuilder_PrepareReturnsGeneratedData(t *testing.T) {
	generated, _, err := new(Builder).Prepare(testConfig(nil))
	if err != nil {
		t.Fatalf("Prepare: %s", err)
	}

	want := map[string]bool{"Fingerprint": true, "ImageAlias": true, "InstanceName": true}
	for _, g := range generated {
		delete(want, g)
	}
	if len(want) != 0 {
		t.Errorf("Prepare did not advertise generated data: %v", want)
	}
}

func TestBuilder_PrepareSurfacesErrors(t *testing.T) {
	if _, _, err := new(Builder).Prepare(testConfig(map[string]interface{}{"image": nil})); err == nil {
		t.Fatal("expected an error when image is unset")
	}
}
