// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package lxd

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
)

func publishState(t *testing.T, server *fakeServer, c *Config) multistep.StateBag {
	t.Helper()

	if _, err := c.Prepare(testConfig(nil)); err != nil {
		t.Fatalf("Prepare: %s", err)
	}

	state := new(multistep.BasicStateBag)
	state.Put("config", c)
	state.Put("ui", testUi())
	state.Put("client", server)

	return state
}

// The fingerprint comes from structured operation metadata. The lxc-based
// plugin scraped it out of human-readable stdout with a regex.
func TestStepPublish_FingerprintFromOperationMetadata(t *testing.T) {
	server := newFakeServer()
	server.fingerprint = "0f1e2d3c4b5a69788796a5b4c3d2e1f0"

	state := publishState(t, server, &Config{})

	if action := (&StepPublish{}).Run(context.Background(), state); action != multistep.ActionContinue {
		t.Fatalf("Run = %v, want continue (err: %v)", action, state.Get("error"))
	}

	got, _ := state.Get("image_fingerprint").(string)
	if got != server.fingerprint {
		t.Errorf("fingerprint = %q, want %q", got, server.fingerprint)
	}
}

// The instance must be stopped before it can be published.
func TestStepPublish_StopsInstanceBeforePublishing(t *testing.T) {
	server := newFakeServer()
	state := publishState(t, server, &Config{})

	if action := (&StepPublish{}).Run(context.Background(), state); action != multistep.ActionContinue {
		t.Fatalf("Run = %v, want continue", action)
	}

	if len(server.stateCalls) != 1 || server.stateCalls[0].Action != "stop" {
		t.Fatalf("state calls = %+v, want a single stop", server.stateCalls)
	}
	if len(server.images) != 1 {
		t.Fatalf("got %d publishes, want 1", len(server.images))
	}
}

func TestStepPublish_SkipPublish(t *testing.T) {
	server := newFakeServer()
	state := publishState(t, server, &Config{SkipPublish: true})

	if action := (&StepPublish{}).Run(context.Background(), state); action != multistep.ActionContinue {
		t.Fatalf("Run = %v, want continue", action)
	}

	if len(server.images) != 0 {
		t.Errorf("published %d images, want 0", len(server.images))
	}
	if _, ok := state.GetOk("image_fingerprint"); ok {
		t.Error("no fingerprint should be recorded when publishing is skipped")
	}
}

// An empty properties map would clear the properties inherited from the source
// image, so it must be left nil when the user set none.
func TestStepPublish_OmitsEmptyProperties(t *testing.T) {
	server := newFakeServer()
	state := publishState(t, server, &Config{})

	(&StepPublish{}).Run(context.Background(), state)

	if server.images[0].Properties != nil {
		t.Errorf("Properties = %v, want nil", server.images[0].Properties)
	}
}

func TestStepPublish_SetsPropertiesAndAliases(t *testing.T) {
	server := newFakeServer()
	config := &Config{
		PublishProperties: map[string]string{"description": "test image"},
		PublishAliases:    []string{"extra", "another"},
	}

	state := publishState(t, server, config)

	(&StepPublish{}).Run(context.Background(), state)

	image := server.images[0]
	if image.Properties["description"] != "test image" {
		t.Errorf("Properties = %v, want description=test image", image.Properties)
	}

	var names []string
	for _, a := range image.Aliases {
		names = append(names, a.Name)
	}

	// output_image comes first, then the extras, with no duplicates.
	want := "packer-foo,extra,another"
	if got := strings.Join(names, ","); got != want {
		t.Errorf("aliases = %q, want %q", got, want)
	}
}

func TestStepPublish_ReuseAliasRemovesConflict(t *testing.T) {
	server := newFakeServer()
	server.aliases["packer-foo"] = true

	state := publishState(t, server, &Config{ReuseAlias: true})

	if action := (&StepPublish{}).Run(context.Background(), state); action != multistep.ActionContinue {
		t.Fatalf("Run = %v, want continue (err: %v)", action, state.Get("error"))
	}

	if len(server.deletedAlias) != 1 || server.deletedAlias[0] != "packer-foo" {
		t.Errorf("deleted aliases = %v, want [packer-foo]", server.deletedAlias)
	}
}

// Without reuse_alias an existing alias is left alone for the server to reject.
func TestStepPublish_WithoutReuseAliasLeavesConflictAlone(t *testing.T) {
	server := newFakeServer()
	server.aliases["packer-foo"] = true

	state := publishState(t, server, &Config{})

	(&StepPublish{}).Run(context.Background(), state)

	if len(server.deletedAlias) != 0 {
		t.Errorf("deleted aliases = %v, want none", server.deletedAlias)
	}
}

func TestStepPublish_AliasNamesMatchPublishedAliases(t *testing.T) {
	config := &Config{OutputImage: "img", PublishAliases: []string{"img", "other"}}

	// output_image duplicated in publish_aliases must not appear twice.
	got := strings.Join((&StepPublish{}).aliasNames(config), ",")
	if got != "img,other" {
		t.Errorf("aliasNames = %q, want \"img,other\"", got)
	}
}
