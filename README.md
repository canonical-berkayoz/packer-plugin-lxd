# Packer Plugin LXD

A [Packer](https://www.packer.io) plugin that builds [LXD](https://canonical.com/lxd)
images by launching an instance from a source image, provisioning it, and
publishing the result as a new image.

Unlike the original `lxc`-based plugin, this one talks to the LXD daemon
directly over its REST and websocket API using the
[LXD Go client](https://github.com/canonical/lxd). The `lxc` command-line tool
is not required.

## Why the API instead of the CLI

Shelling out to `lxc` meant every operation passed through a local shell, then
`lxc`, then a shell inside the instance. Going straight to the API removes that
layering and the defects that came with it:

| | `lxc` shell-out | LXD API |
| --- | --- | --- |
| Provisioners holding stdin open (Ansible) | Hang; need an `ansible_connection` workaround | Work as-is |
| Command quoting | Hand-rolled escaping through three shells | Argument vector, nothing to escape |
| Exit codes | Recovered from `syscall.WaitStatus` | Reported by the API |
| Image fingerprint | Regex-scraped from human-readable output | Structured operation metadata |
| Instance readiness | A fixed `init_sleep` | Polled until it really is ready |
| `download_dir` | Not implemented | Implemented over SFTP |

## Installation

```hcl
packer {
  required_plugins {
    lxd = {
      version = ">= 0.0.1"
      source  = "github.com/canonical/lxd"
    }
  }
}
```

Then run `packer init`.

## Usage

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

See [`docs/builders/lxd.mdx`](docs/builders/lxd.mdx) for the full configuration
reference and [`example/`](example/) for a runnable template.

## Requirements

- [Go](https://go.dev) >= 1.25.7
- [Packer](https://www.packer.io/docs/install) >= v1.10.2
- A reachable LXD daemon. If `lxc version` reports the server as unreachable,
  add yourself to the `lxd` group:
  ```shell
  sudo usermod -aG lxd "$USER"   # then log out and back in, or run: newgrp lxd
  ```

## Development

Build and install the plugin locally:

```shell
make dev
```

Run the unit tests:

```shell
make test
```

Run the acceptance tests. These launch real instances and publish real images,
so they need a working LXD daemon:

```shell
make testacc
```

Regenerate the HCL2 spec and the docs after changing the config struct:

```shell
make generate
```

## License

This repository is covered by the [AGPL-3.0](LICENSE).
It is based on [packer-plugin-scaffolding](https://github.com/hashicorp/packer-plugin-scaffolding), which is covered by the [MPL-2.0](LICENSE.MPL-2.0).
