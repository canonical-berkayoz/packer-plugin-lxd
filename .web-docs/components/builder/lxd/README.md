Type: `lxd`
Artifact BuilderId: `lxd`

The LXD Packer builder builds LXD images by launching an instance from a source
image, running Packer's provisioners inside it, and publishing the result as a
new image.

The builder communicates with the LXD daemon directly over its REST and
websocket API using the [LXD Go client](https://github.com/canonical/lxd). The
`lxc` command-line tool does **not** need to be installed. Commands run through
the instance exec API and files move over the instance's SFTP endpoint, so
provisioner commands are never re-parsed by an intermediate shell.

Both containers and virtual machines are supported. Provisioning a virtual
machine requires `lxd-agent` to be running inside the guest, since that is what
provides the exec and file-transfer endpoints.

## Basic Example

```hcl
source "lxd" "example" {
  image        = "ubuntu:24.04"
  output_image = "ubuntu-with-nginx"
}

build {
  sources = ["source.lxd.example"]

  provisioner "shell" {
    inline = ["apt-get update", "apt-get install -y nginx"]
  }
}
```

## Configuration Reference

### Required

<!-- Code generated from the comments of the Config struct in builder/lxd/config.go; DO NOT EDIT MANUALLY -->

<!-- Code generated from the comments of the Config struct in builder/lxd/config.go; DO NOT EDIT MANUALLY -->

- `image` (string) - The source image to use when creating the build instance. This can be a
  local or remote image, given as an alias or a fingerprint. Remotes are
  resolved the same way the `lxc` CLI resolves them, so both the built-in
  remotes (`images:`, `ubuntu:`, `ubuntu-daily:`, ...) and any remote
  configured in your LXD client config may be used.
  E.g. `my-base-image`, `ubuntu:24.04`, `images:alpine/edge`, `08fababf6f27`.

<!-- End of code generated from the comments of the Config struct in builder/lxd/config.go; -->


### Optional

<!-- Code generated from the comments of the Config struct in builder/lxd/config.go; DO NOT EDIT MANUALLY -->

- `image_remote_url` (string) - Explicit image server URL, bypassing remote-name resolution. When set,
  `image` is treated as an alias or fingerprint on this server.

- `image_remote_protocol` (string) - Protocol of `image_remote_url`; either `simplestreams` or `lxd`.
  Defaults to `simplestreams`.

- `container_name` (string) - The name of the build instance. Defaults to `packer-{{PackerBuildName}}`.

- `output_image` (string) - The name of the output artifact. Defaults to `container_name`.

- `publish_remote_name` (string) - The (optional) name of the LXD remote on which to publish the image.

- `remote_name` (string) - The name of a remote from your LXD client config to build on. Mutually
  exclusive with `daemon_address`. Defaults to the client config's default
  remote, or the local unix socket.

- `daemon_address` (string) - Address of the LXD daemon to build on: either a unix socket path or an
  `https://host:8443` URL. Mutually exclusive with `remote_name`.

- `client_cert` (string) - Path to a PEM client certificate used to authenticate to the daemon.

- `client_key` (string) - Path to the PEM client key matching `client_cert`.

- `server_cert` (string) - Path to the daemon's PEM server certificate, for pinning.

- `insecure_skip_verify` (bool) - Do not verify the daemon's TLS certificate. Defaults to false.

- `project` (string) - The LXD project to build in. Defaults to the remote's project.

- `target` (string) - The cluster member to target when building against an LXD cluster.

- `profile` (string) - A single profile to apply to the build instance. Defaults to `default`.
  Use `profiles` to apply more than one.

- `profiles` ([]string) - The list of profiles to apply to the build instance. Mutually exclusive
  with `profile`.

- `launch_config` (map[string]string) - Key/value pairs set as instance configuration keys on the build instance,
  equivalent to `lxc launch --config`. Defaults to empty.

- `launch_devices` (map[string]map[string]string) - Devices to attach to the build instance, keyed by device name.

- `virtual_machine` (bool) - Create a virtual-machine image instead of a container image; defaults to
  false. Equivalent to setting `instance_type` to `virtual-machine`.

- `instance_type` (string) - The type of instance to build: `container` or `virtual-machine`.
  Defaults to `container`. Mutually exclusive with `virtual_machine`.

- `ephemeral` (bool) - Make the build instance ephemeral. Defaults to false; an ephemeral
  instance cannot be stopped and published.

- `boot_timeout` (duration string | ex: "1h5m2s") - How long to wait for the instance to become ready to run commands.
  Defaults to `2m`.

- `wait_for_network` (bool) - Additionally wait for the instance to be assigned a non-loopback IPv4
  address before provisioning. Defaults to false.

- `publish_properties` (map[string]string) - Key/value pairs set as properties on the published image. Most commonly
  used to set `description`.

- `publish_aliases` ([]string) - Additional aliases to create for the published image, alongside
  `output_image`.

- `publish_public` (bool) - Mark the published image public. Defaults to false.

- `compression_algorithm` (string) - Compression algorithm to use when publishing, e.g. `gzip`, `xz`, or
  `none`. Defaults to the server's setting.

- `reuse_alias` (bool) - Delete any pre-existing alias that conflicts with the aliases being
  created, instead of failing. Defaults to false.

- `skip_publish` (bool) - Skip publishing the image. The build instance is left running so it can
  be inspected. Defaults to false.

- `exec_environment` (map[string]string) - Environment variables set for every command run by provisioners.

- `exec_user` (uint32) - The uid to run provisioner commands as. Defaults to 0 (root).

- `exec_group` (uint32) - The gid to run provisioner commands as. Defaults to 0 (root).

- `exec_cwd` (string) - The working directory for provisioner commands. Defaults to the image's
  configured working directory.

- `command_wrapper` (string) - Deprecated and ignored. The plugin talks to the LXD API directly, so
  there is no shell command to wrap.

- `init_sleep` (string) - Deprecated and ignored. Superseded by a real readiness probe; see
  `boot_timeout`.

<!-- End of code generated from the comments of the Config struct in builder/lxd/config.go; -->


## Source Images

`image` accepts the same `remote:name` forms the `lxc` CLI accepts. The built-in
remotes (`images:`, `ubuntu:`, `ubuntu-daily:`, `ubuntu-minimal:`, `local:`) are
always available, and any remote configured in your LXD client configuration —
`$LXD_CONF`, `~/snap/lxd/common/config`, or `~/.config/lxc` — is resolved too,
including its client certificates.

```hcl
image = "ubuntu:24.04"          # a built-in remote
image = "images:alpine/edge"    # another built-in remote
image = "my-remote:base-image"  # a remote from your lxc config
image = "08fababf6f27"          # a fingerprint on the build daemon
```

To bypass remote resolution entirely, set `image_remote_url` and treat `image`
as an alias or fingerprint on that server.

## Connecting to the Daemon

By default the builder connects to the local daemon over its unix socket,
honouring `$LXD_SOCKET` and `$LXD_DIR`. To build against a different daemon, use
either `remote_name` (a remote from your LXD client configuration) or
`daemon_address` together with `client_cert` and `client_key`.

```hcl
source "lxd" "remote-build" {
  image          = "ubuntu:24.04"
  daemon_address = "https://lxd.example.com:8443"
  client_cert    = "/home/me/.config/lxc/client.crt"
  client_key     = "/home/me/.config/lxc/client.key"
  project        = "packer"
}
```

## Virtual Machines

```hcl
source "lxd" "vm" {
  image         = "ubuntu:24.04"
  instance_type = "virtual-machine"
  output_image  = "ubuntu-vm"

  launch_config = {
    "limits.cpu"    = "2"
    "limits.memory" = "2GiB"
  }
}
```

The source image must contain `lxd-agent`; on Ubuntu images this comes from the
`lxd-agent-loader` package. Until the agent is up, the instance cannot run
commands, so the builder waits for it — raise `boot_timeout` if your guest is
slow to boot.

## Migrating from the `lxc`-based plugin

Templates written for the previous plugin continue to work. Two options are
accepted but ignored, and produce a warning:

- `command_wrapper` — there is no shell command to wrap. Use `remote_name` or
  `daemon_address` to build against a remote daemon instead.
- `init_sleep` — the builder now waits for the instance to actually become ready
  rather than sleeping a fixed number of seconds. Use `boot_timeout` to bound
  that wait.

Behaviour that changed for the better:

- Provisioner commands are passed to the API as an argument vector, so quoting
  and `$` no longer need escaping through nested shells.
- Provisioners that hold stdin open (notably Ansible) no longer hang. The
  `ansible_connection=community.general.lxd` workaround is not needed.
- `download_dir` is implemented.
- With `skip_publish`, the artifact reports the instance it left running instead
  of the placeholder id `0`.
