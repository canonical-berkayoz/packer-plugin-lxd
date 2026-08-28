// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package lxd

import (
	"strings"
	"testing"

	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

func TestArtifact_ImplementsArtifact(t *testing.T) {
	var raw interface{} = new(Artifact)
	if _, ok := raw.(packersdk.Artifact); !ok {
		t.Fatal("Artifact must implement packersdk.Artifact")
	}
}

func TestArtifact_Id(t *testing.T) {
	published := &Artifact{Fingerprint: "abc123", InstanceName: "packer-foo"}
	if got := published.Id(); got != "abc123" {
		t.Errorf("Id() = %q, want abc123", got)
	}

	// With skip_publish there is no image; the old plugin reported the literal
	// string "0" here, which identified nothing.
	unpublished := &Artifact{InstanceName: "packer-foo"}
	if got := unpublished.Id(); got != "instance:packer-foo" {
		t.Errorf("Id() = %q, want instance:packer-foo", got)
	}
	if unpublished.Id() == "0" {
		t.Error("unpublished artifact must not report the placeholder id \"0\"")
	}
}

func TestArtifact_String(t *testing.T) {
	a := &Artifact{Fingerprint: "abc123", Aliases: []string{"my-image"}}
	if got := a.String(); !strings.Contains(got, "abc123") || !strings.Contains(got, "my-image") {
		t.Errorf("String() = %q, want it to mention the fingerprint and alias", got)
	}

	u := &Artifact{InstanceName: "packer-foo"}
	if got := u.String(); !strings.Contains(got, "packer-foo") {
		t.Errorf("String() = %q, want it to mention the instance", got)
	}
}

func TestArtifact_BuilderIdIsStable(t *testing.T) {
	// Post-processors key off this; it matches the lxc-based plugin.
	if BuilderId != "lxd" {
		t.Errorf("BuilderId = %q, want lxd", BuilderId)
	}
}

func TestArtifact_FilesIsEmpty(t *testing.T) {
	if files := (&Artifact{}).Files(); len(files) != 0 {
		t.Errorf("Files() = %v, want none", files)
	}
}

func TestArtifact_State(t *testing.T) {
	a := &Artifact{StateData: map[string]interface{}{"generated_data": "x"}}
	if got := a.State("generated_data"); got != "x" {
		t.Errorf("State() = %v, want x", got)
	}
}

func TestArtifact_DestroyDeletesImageAndAliases(t *testing.T) {
	server := newFakeServer()
	server.aliases["my-image"] = true

	a := &Artifact{
		Fingerprint: "abc123",
		Aliases:     []string{"my-image"},
		client:      server,
	}

	if err := a.Destroy(); err != nil {
		t.Fatalf("Destroy: %s", err)
	}

	if len(server.deletedAlias) != 1 || server.deletedAlias[0] != "my-image" {
		t.Errorf("deleted aliases = %v, want [my-image]", server.deletedAlias)
	}
}

// Nothing was published, so there is nothing to destroy.
func TestArtifact_DestroyUnpublishedIsANoop(t *testing.T) {
	if err := (&Artifact{InstanceName: "packer-foo"}).Destroy(); err != nil {
		t.Fatalf("Destroy: %s", err)
	}
}
