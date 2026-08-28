// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package lxd

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	lxdclient "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

func TestCommunicator_ImplementsCommunicator(t *testing.T) {
	var raw interface{} = new(Communicator)
	if _, ok := raw.(packersdk.Communicator); !ok {
		t.Fatal("Communicator must implement packersdk.Communicator")
	}
}

// blockingReader never returns and never reaches EOF, standing in for the
// Ansible provisioner's ssh.Channel.
type blockingReader struct{ done chan struct{} }

func (b *blockingReader) Read(p []byte) (int, error) {
	<-b.done
	return 0, io.EOF
}

// The lxc-based plugin passed the provisioner's stdin straight to a local
// process and blocked in Wait until that reader hit EOF, which the Ansible
// adapter's channel never does. Nothing in the native path may reintroduce
// that: a command must report its exit status even while stdin stays open.
func TestCommunicator_StartDoesNotBlockOnOpenStdin(t *testing.T) {
	reader := &blockingReader{done: make(chan struct{})}
	defer close(reader.done)

	server := newFakeServer()
	server.execFn = func(api.InstanceExecPost, *lxdclient.InstanceExecArgs) (int, error) {
		return 0, nil
	}

	comm := &Communicator{Client: server, InstanceName: "test"}
	cmd := &packersdk.RemoteCmd{Command: "true", Stdin: reader}

	if err := comm.Start(context.Background(), cmd); err != nil {
		t.Fatalf("Start: %s", err)
	}

	exited := make(chan int, 1)
	go func() { exited <- cmd.Wait() }()

	select {
	case status := <-exited:
		if status != 0 {
			t.Fatalf("exit status = %d, want 0", status)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("command did not exit while stdin was still open")
	}
}

// A nil Stdin must still terminate: it is replaced with an empty reader so the
// stdin stream closes immediately.
func TestCommunicator_StartWithNilStdin(t *testing.T) {
	server := newFakeServer()
	server.execFn = func(api.InstanceExecPost, *lxdclient.InstanceExecArgs) (int, error) {
		return 0, nil
	}

	comm := &Communicator{Client: server, InstanceName: "test"}
	cmd := &packersdk.RemoteCmd{Command: "true"}

	if err := comm.Start(context.Background(), cmd); err != nil {
		t.Fatalf("Start: %s", err)
	}

	done := make(chan int, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("command with nil stdin did not exit")
	}
}

func TestCommunicator_StartReportsExitStatus(t *testing.T) {
	for _, want := range []int{0, 1, 42, 127} {
		server := newFakeServer()
		server.execFn = func(api.InstanceExecPost, *lxdclient.InstanceExecArgs) (int, error) {
			return want, nil
		}

		comm := &Communicator{Client: server, InstanceName: "test"}
		cmd := &packersdk.RemoteCmd{Command: "exit"}

		if err := comm.Start(context.Background(), cmd); err != nil {
			t.Fatalf("Start: %s", err)
		}

		if got := cmd.Wait(); got != want {
			t.Errorf("exit status = %d, want %d", got, want)
		}
	}
}

// Output must be fully flushed before the exit status is reported, or
// provisioners see truncated logs.
func TestCommunicator_StartDrainsOutput(t *testing.T) {
	payload := strings.Repeat("a", 256*1024)

	server := newFakeServer()
	server.execFn = func(_ api.InstanceExecPost, args *lxdclient.InstanceExecArgs) (int, error) {
		if _, err := io.WriteString(args.Stdout, payload); err != nil {
			return 1, err
		}
		if _, err := io.WriteString(args.Stderr, payload); err != nil {
			return 1, err
		}

		return 0, nil
	}

	var stdout, stderr bytes.Buffer
	comm := &Communicator{Client: server, InstanceName: "test"}
	cmd := &packersdk.RemoteCmd{Command: "cat", Stdout: &stdout, Stderr: &stderr}

	if err := comm.Start(context.Background(), cmd); err != nil {
		t.Fatalf("Start: %s", err)
	}
	cmd.Wait()

	if stdout.Len() != len(payload) {
		t.Errorf("stdout = %d bytes, want %d", stdout.Len(), len(payload))
	}
	if stderr.Len() != len(payload) {
		t.Errorf("stderr = %d bytes, want %d", stderr.Len(), len(payload))
	}
}

// Commands are handed to the API as an argv, with no host shell in between, so
// there is nothing to quote and nothing to inject through.
func TestCommunicator_StartPassesCommandAsArgv(t *testing.T) {
	command := `echo "it's $HOME" && printf '%s\n' 'a b'`

	server := newFakeServer()
	server.execFn = func(api.InstanceExecPost, *lxdclient.InstanceExecArgs) (int, error) {
		return 0, nil
	}

	comm := &Communicator{
		Client:       server,
		InstanceName: "test",
		Environment:  map[string]string{"FOO": "bar"},
		User:         1000,
		Group:        1000,
		Cwd:          "/srv",
	}
	cmd := &packersdk.RemoteCmd{Command: command}

	if err := comm.Start(context.Background(), cmd); err != nil {
		t.Fatalf("Start: %s", err)
	}
	cmd.Wait()

	if len(server.execCalls) != 1 {
		t.Fatalf("got %d exec calls, want 1", len(server.execCalls))
	}

	req := server.execCalls[0]
	want := []string{"/bin/sh", "-c", command}

	if len(req.Command) != len(want) {
		t.Fatalf("command = %q, want %q", req.Command, want)
	}
	for i := range want {
		if req.Command[i] != want[i] {
			t.Errorf("command[%d] = %q, want %q", i, req.Command[i], want[i])
		}
	}

	if req.WaitForWS != true || req.Interactive != false {
		t.Errorf("WaitForWS=%v Interactive=%v, want true/false", req.WaitForWS, req.Interactive)
	}
	if req.Environment["FOO"] != "bar" {
		t.Errorf("Environment = %v, want FOO=bar", req.Environment)
	}
	if req.User != 1000 || req.Group != 1000 || req.Cwd != "/srv" {
		t.Errorf("User/Group/Cwd = %d/%d/%q, want 1000/1000/\"/srv\"", req.User, req.Group, req.Cwd)
	}
}
