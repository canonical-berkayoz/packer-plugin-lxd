# LXD builder example

Builds an Ubuntu 24.04 container image with the `lxd` builder and publishes it
under the alias `packer-lxd-example`.

Requires a running LXD daemon that your user can reach. If `lxc version` reports
the server as unreachable, add yourself to the `lxd` group:

```shell
sudo usermod -aG lxd "$USER"   # then log out and back in, or run: newgrp lxd
```

Run it:

```shell
packer init .
packer validate .
packer build .
```

Then clean up:

```shell
lxc image delete packer-lxd-example
```
