// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package lxd

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	lxdclient "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/pkg/sftp"
)

// Communicator runs provisioner commands and moves files through the LXD API:
// commands over the exec websocket, files over the instance's SFTP endpoint.
// No shell is involved on the host and the `lxc` binary is never invoked.
type Communicator struct {
	Client       instanceServer
	InstanceName string

	Environment map[string]string
	User        uint32
	Group       uint32
	Cwd         string

	sftpOnce sync.Once
	sftpConn *sftp.Client
	sftpErr  error
}

var _ packersdk.Communicator = new(Communicator)

// Start runs a command inside the instance and reports its real exit status.
func (c *Communicator) Start(ctx context.Context, cmd *packersdk.RemoteCmd) error {
	log.Printf("[INFO] LXD exec in %s: %s", c.InstanceName, cmd.Command)

	req := api.InstanceExecPost{
		Command:     []string{"/bin/sh", "-c", cmd.Command},
		WaitForWS:   true,
		Interactive: false,
		Environment: c.Environment,
		User:        c.User,
		Group:       c.Group,
		Cwd:         c.Cwd,
	}

	stdin := cmd.Stdin
	if stdin == nil {
		// LXD copies from this reader into the stdin websocket and closes the
		// socket when it hits EOF. Handing it a nil reader would leave stdin
		// open forever and hang any command that reads to EOF, which is the
		// failure the lxc-based plugin hit with Ansible.
		stdin = strings.NewReader("")
	}

	dataDone := make(chan bool)
	args := &lxdclient.InstanceExecArgs{
		Stdin:    stdin,
		Stdout:   cmd.Stdout,
		Stderr:   cmd.Stderr,
		DataDone: dataDone,
	}

	// The exec API always wires up all three streams; give it sinks rather than
	// nil writers so it never writes into a nil interface.
	if args.Stdout == nil {
		args.Stdout = io.Discard
	}
	if args.Stderr == nil {
		args.Stderr = io.Discard
	}

	op, err := c.Client.ExecInstance(c.InstanceName, req, args)
	if err != nil {
		return fmt.Errorf("starting command in instance %s: %w", c.InstanceName, err)
	}

	go func() {
		exitStatus := 0

		if err := op.WaitContext(ctx); err != nil {
			log.Printf("[ERROR] LXD exec failed in %s: %s", c.InstanceName, err)
			exitStatus = 1
		} else {
			// The operation carries the command's real exit code; JSON decoding
			// gives it to us as a float64.
			if ret, ok := op.Get().Metadata["return"].(float64); ok {
				exitStatus = int(ret)
			}
		}

		// Output is still in flight until LXD signals the streams are drained.
		// Reporting the exit status before this would truncate the output the
		// provisioner sees.
		<-dataDone

		log.Printf("[INFO] LXD exec in %s exited with %d: %s", c.InstanceName, exitStatus, cmd.Command)
		cmd.SetExited(exitStatus)
	}()

	return nil
}

// sftpClient lazily opens, and then reuses, an SFTP connection to the instance.
func (c *Communicator) sftpClient() (*sftp.Client, error) {
	c.sftpOnce.Do(func() {
		c.sftpConn, c.sftpErr = c.Client.GetInstanceFileSFTP(c.InstanceName)
		if c.sftpErr != nil {
			c.sftpErr = fmt.Errorf("opening SFTP connection to instance %s "+
				"(virtual machines need lxd-agent running in the guest): %w",
				c.InstanceName, c.sftpErr)
		}
	})

	return c.sftpConn, c.sftpErr
}

// Close releases the SFTP connection, if one was opened.
func (c *Communicator) Close() error {
	if c.sftpConn == nil {
		return nil
	}

	return c.sftpConn.Close()
}

// Upload writes a single file into the instance.
func (c *Communicator) Upload(dst string, r io.Reader, fi *os.FileInfo) error {
	client, err := c.sftpClient()
	if err != nil {
		return err
	}

	// Uploading to an existing directory means "into" that directory. Paths
	// inside the instance are always POSIX, so join with path, not filepath,
	// which would emit backslashes when Packer runs on Windows.
	if st, err := client.Stat(dst); err == nil && st.IsDir() {
		if fi == nil {
			return fmt.Errorf("upload destination %s is a directory and no source file name is known", dst)
		}

		dst = path.Join(dst, (*fi).Name())
	}

	if dir := path.Dir(dst); dir != "" && dir != "." {
		if err := client.MkdirAll(dir); err != nil {
			return fmt.Errorf("creating directory %s in instance: %w", dir, err)
		}
	}

	log.Printf("[INFO] Uploading to %s in instance %s", dst, c.InstanceName)

	f, err := client.Create(dst)
	if err != nil {
		return fmt.Errorf("creating %s in instance: %w", dst, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("writing %s in instance: %w", dst, err)
	}

	if fi != nil {
		if err := client.Chmod(dst, (*fi).Mode().Perm()); err != nil {
			return fmt.Errorf("setting mode on %s in instance: %w", dst, err)
		}
	}

	return f.Close()
}

// UploadDir copies a local directory tree into the instance. A src with a
// trailing slash uploads the directory's contents; without one, the directory
// itself is created inside dst.
func (c *Communicator) UploadDir(dst string, src string, exclude []string) error {
	client, err := c.sftpClient()
	if err != nil {
		return err
	}

	if !strings.HasSuffix(src, "/") {
		dst = path.Join(dst, filepath.Base(src))
	}
	src = strings.TrimSuffix(src, "/")

	log.Printf("[INFO] Uploading dir %s to %s in instance %s", src, dst, c.InstanceName)

	return filepath.Walk(src, func(localPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, localPath)
		if err != nil {
			return err
		}

		if isExcluded(rel, exclude) {
			if info.IsDir() {
				return filepath.SkipDir
			}

			return nil
		}

		remotePath := dst
		if rel != "." {
			remotePath = path.Join(dst, filepath.ToSlash(rel))
		}

		switch {
		case info.IsDir():
			if err := client.MkdirAll(remotePath); err != nil {
				return fmt.Errorf("creating directory %s in instance: %w", remotePath, err)
			}

			return client.Chmod(remotePath, info.Mode().Perm())

		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(localPath)
			if err != nil {
				return err
			}

			// A symlink may already exist from an earlier run; replace it.
			_ = client.Remove(remotePath)

			return client.Symlink(target, remotePath)

		case !info.Mode().IsRegular():
			log.Printf("[WARN] Skipping non-regular file %s", localPath)
			return nil
		}

		f, err := os.Open(localPath)
		if err != nil {
			return err
		}
		defer f.Close()

		fi := info
		return c.Upload(remotePath, f, &fi)
	})
}

// Download copies a single file out of the instance.
func (c *Communicator) Download(src string, w io.Writer) error {
	client, err := c.sftpClient()
	if err != nil {
		return err
	}

	log.Printf("[INFO] Downloading %s from instance %s", src, c.InstanceName)

	f, err := client.Open(src)
	if err != nil {
		return fmt.Errorf("opening %s in instance: %w", src, err)
	}
	defer f.Close()

	if _, err := io.Copy(w, f); err != nil {
		return fmt.Errorf("reading %s from instance: %w", src, err)
	}

	return nil
}

// DownloadDir copies a directory tree out of the instance. The lxc-based plugin
// never implemented this.
func (c *Communicator) DownloadDir(src string, dst string, exclude []string) error {
	client, err := c.sftpClient()
	if err != nil {
		return err
	}

	if !strings.HasSuffix(src, "/") {
		dst = filepath.Join(dst, path.Base(src))
	}
	src = strings.TrimSuffix(src, "/")

	log.Printf("[INFO] Downloading dir %s from instance %s to %s", src, c.InstanceName, dst)

	walker := client.Walk(src)
	for walker.Step() {
		if err := walker.Err(); err != nil {
			return err
		}

		info := walker.Stat()

		rel, err := filepath.Rel(src, filepath.FromSlash(walker.Path()))
		if err != nil {
			return err
		}

		if isExcluded(filepath.ToSlash(rel), exclude) {
			if info.IsDir() {
				walker.SkipDir()
			}

			continue
		}

		localPath := dst
		if rel != "." {
			localPath = filepath.Join(dst, rel)
		}

		if info.IsDir() {
			if err := os.MkdirAll(localPath, info.Mode().Perm()); err != nil {
				return err
			}

			continue
		}

		if !info.Mode().IsRegular() {
			log.Printf("[WARN] Skipping non-regular file %s in instance", walker.Path())
			continue
		}

		if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
			return err
		}

		local, err := os.OpenFile(localPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
		if err != nil {
			return err
		}

		err = c.Download(walker.Path(), local)
		if closeErr := local.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
	}

	return nil
}

// isExcluded reports whether a path relative to the transfer root matches any
// of the caller's exclude patterns.
func isExcluded(rel string, exclude []string) bool {
	if rel == "." {
		return false
	}

	base := path.Base(rel)
	for _, pattern := range exclude {
		if pattern == "" {
			continue
		}

		if pattern == rel || pattern == base {
			return true
		}

		if ok, err := path.Match(pattern, rel); err == nil && ok {
			return true
		}
		if ok, err := path.Match(pattern, base); err == nil && ok {
			return true
		}
	}

	return false
}
