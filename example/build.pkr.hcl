packer {
  required_plugins {
    lxd = {
      version = ">= 0.0.1"
      source  = "github.com/canonical/lxd"
    }
  }
}

source "lxd" "basic-example" {
  image        = "ubuntu:24.04"
  output_image = "packer-lxd-example"
  reuse_alias  = true

  publish_properties = {
    description = "Built by Packer via the LXD API"
  }
}

build {
  sources = ["source.lxd.basic-example"]

  provisioner "shell" {
    inline = [
      "echo 'hello from inside the instance'",
      # Quotes and $ reach the guest untouched: the command is passed to the
      # LXD API as an argument vector, not re-parsed by intermediate shells.
      "echo \"HOME is $HOME\"",
    ]
  }

  provisioner "file" {
    content     = "provisioned by packer\n"
    destination = "/etc/packer-example"
  }
}
