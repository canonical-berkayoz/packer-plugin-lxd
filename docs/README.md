The LXD plugin builds [LXD](https://canonical.com/lxd) images by launching an
instance from a source image, running Packer's provisioners inside it, and
publishing the result as a new image.

It talks to the LXD daemon directly over its REST and websocket API using the
[LXD Go client](https://github.com/canonical/lxd), so the `lxc` command-line
tool is not required.

### Installation

To install this plugin, copy and paste this code into your Packer configuration, then run [`packer init`](https://www.packer.io/docs/commands/init).

```hcl
packer {
  required_plugins {
    lxd = {
      source  = "github.com/canonical-berkayoz/lxd"
      version = ">= 0.0.1"
    }
  }
}
```

Alternatively, you can use `packer plugins install` to manage installation of this plugin.

```sh
$ packer plugins install github.com/canonical-berkayoz/lxd
```

### Components

#### Builders

- [lxd](/packer/integrations/canonical/lxd/latest/components/builder/lxd) - Launches an
  instance from a source image, provisions it, and publishes the running
  instance as a new LXD image.
