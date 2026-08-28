// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package lxd

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	lxdclient "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	"github.com/gorilla/websocket"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/pkg/sftp"
)

// fakeOperation is a canned lxdclient.Operation.
type fakeOperation struct {
	metadata map[string]interface{}
	err      error
}

func (o *fakeOperation) AddHandler(func(api.Operation)) (*lxdclient.EventTarget, error) {
	return nil, nil
}
func (o *fakeOperation) Cancel() error { return nil }
func (o *fakeOperation) Get() api.Operation {
	return api.Operation{Metadata: o.metadata}
}
func (o *fakeOperation) GetWebsocket(string) (*websocket.Conn, error) { return nil, nil }
func (o *fakeOperation) RemoveHandler(*lxdclient.EventTarget) error   { return nil }
func (o *fakeOperation) Refresh() error                               { return nil }
func (o *fakeOperation) Wait() error                                  { return o.err }
func (o *fakeOperation) WaitContext(context.Context) error            { return o.err }

// fakeServer is a stand-in for a real LXD connection. It models exec closely
// enough to exercise the communicator's stdin, output and exit-status handling
// without a daemon.
type fakeServer struct {
	mu sync.Mutex

	// execFn, when set, produces the command's behaviour.
	execFn func(req api.InstanceExecPost, args *lxdclient.InstanceExecArgs) (int, error)

	execCalls  []api.InstanceExecPost
	stateCalls []api.InstanceStatePut
	images     []api.ImagesPost

	instanceState *api.InstanceState
	aliases       map[string]bool
	deletedAlias  []string
	publishErr    error
	fingerprint   string
}

func newFakeServer() *fakeServer {
	return &fakeServer{
		instanceState: &api.InstanceState{StatusCode: api.Running},
		aliases:       map[string]bool{},
		fingerprint:   "abc123def456",
	}
}

func (f *fakeServer) GetServer() (*api.Server, string, error) { return &api.Server{}, "", nil }
func (f *fakeServer) HasExtension(string) bool                { return true }

func (f *fakeServer) CreateInstanceFromImage(lxdclient.ImageServer, api.Image, api.InstancesPost) (lxdclient.RemoteOperation, error) {
	return nil, fmt.Errorf("not implemented")
}

func (f *fakeServer) GetInstanceState(string) (*api.InstanceState, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.instanceState, "", nil
}

func (f *fakeServer) UpdateInstanceState(_ string, state api.InstanceStatePut, _ string) (lxdclient.Operation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.stateCalls = append(f.stateCalls, state)

	return &fakeOperation{}, nil
}

func (f *fakeServer) DeleteInstance(string, bool) (lxdclient.Operation, error) {
	return &fakeOperation{}, nil
}

// ExecInstance mirrors what the real client does: it copies stdin to the
// (discarded) remote end, writes whatever the command produces, then signals
// DataDone once the streams are drained.
func (f *fakeServer) ExecInstance(_ string, req api.InstanceExecPost, args *lxdclient.InstanceExecArgs) (lxdclient.Operation, error) {
	f.mu.Lock()
	f.execCalls = append(f.execCalls, req)
	execFn := f.execFn
	f.mu.Unlock()

	// Stdin is drained on its own goroutine and is deliberately NOT waited on.
	// This mirrors the real client, which closes DataDone once the *output*
	// streams finish: "Handle stdin finish, but don't wait for it if output
	// channels have all finished" (client/lxd_instances.go). A reader that
	// never reaches EOF must therefore never hold up the command.
	if args.Stdin != nil {
		go func() { _, _ = io.Copy(io.Discard, args.Stdin) }()
	}

	ret := 0
	var err error

	if execFn != nil {
		ret, err = execFn(req, args)
	}

	// Output is drained; signal the caller it is safe to report exit status.
	close(args.DataDone)

	return &fakeOperation{
		metadata: map[string]interface{}{"return": float64(ret)},
		err:      err,
	}, nil
}

func (f *fakeServer) GetInstanceFileSFTP(string) (*sftp.Client, error) {
	return nil, fmt.Errorf("sftp not available in fake")
}

func (f *fakeServer) CreateImage(image api.ImagesPost, _ *lxdclient.ImageCreateArgs) (lxdclient.Operation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.images = append(f.images, image)

	if f.publishErr != nil {
		return nil, f.publishErr
	}

	return &fakeOperation{metadata: map[string]interface{}{"fingerprint": f.fingerprint}}, nil
}

func (f *fakeServer) DeleteImage(string) (lxdclient.Operation, error) {
	return &fakeOperation{}, nil
}

func (f *fakeServer) CreateImageAlias(alias api.ImageAliasesPost) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.aliases[alias.Name] {
		return fmt.Errorf("alias %s already exists", alias.Name)
	}
	f.aliases[alias.Name] = true

	return nil
}

func (f *fakeServer) DeleteImageAlias(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.deletedAlias = append(f.deletedAlias, name)
	delete(f.aliases, name)

	return nil
}

func (f *fakeServer) GetImageAlias(name string) (*api.ImageAliasesEntry, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if !f.aliases[name] {
		return nil, "", fmt.Errorf("alias %s not found", name)
	}

	return &api.ImageAliasesEntry{}, "", nil
}

var _ instanceServer = new(fakeServer)

// testUi is a Ui that writes nowhere, for tests that only care about behaviour.
func testUi() packersdk.Ui {
	return &packersdk.BasicUi{
		Reader: strings.NewReader(""),
		Writer: io.Discard,
	}
}
