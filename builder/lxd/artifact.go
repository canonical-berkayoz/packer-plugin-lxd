// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package lxd

import (
	"fmt"
	"log"
)

// Artifact is the LXD image produced by a build. When skip_publish is set there
// is no image, and the artifact instead identifies the instance left running.
type Artifact struct {
	// Fingerprint of the published image; empty when skip_publish is set.
	Fingerprint string
	// Aliases created for the published image.
	Aliases []string
	// Name of the build instance.
	InstanceName string

	client    instanceServer
	StateData map[string]interface{}
}

func (*Artifact) BuilderId() string {
	return BuilderId
}

// Files returns nil: the artifact lives in the LXD image store, not on disk.
func (*Artifact) Files() []string {
	return nil
}

func (a *Artifact) Id() string {
	if a.Fingerprint == "" {
		return fmt.Sprintf("instance:%s", a.InstanceName)
	}

	return a.Fingerprint
}

func (a *Artifact) String() string {
	if a.Fingerprint == "" {
		return fmt.Sprintf("instance: %s (not published)", a.InstanceName)
	}

	if len(a.Aliases) > 0 {
		return fmt.Sprintf("image: %s (aliases: %v)", a.Fingerprint, a.Aliases)
	}

	return fmt.Sprintf("image: %s", a.Fingerprint)
}

func (a *Artifact) State(name string) interface{} {
	return a.StateData[name]
}

// Destroy removes the published image and the aliases pointing at it.
func (a *Artifact) Destroy() error {
	if a.Fingerprint == "" || a.client == nil {
		return nil
	}

	for _, alias := range a.Aliases {
		if err := a.client.DeleteImageAlias(alias); err != nil {
			log.Printf("[WARN] Error deleting image alias %s: %s", alias, err)
		}
	}

	op, err := a.client.DeleteImage(a.Fingerprint)
	if err == nil {
		err = op.Wait()
	}
	if err != nil {
		return fmt.Errorf("deleting image %s: %w", a.Fingerprint, err)
	}

	return nil
}
