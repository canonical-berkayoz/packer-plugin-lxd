source "lxd" "basic-example" {
  image        = "ubuntu:24.04"
  output_image = "packer-lxd-acc-test"
  reuse_alias  = true
}

build {
  sources = ["source.lxd.basic-example"]

  provisioner "shell" {
    inline = ["echo packer-lxd-acc-test-marker > /etc/packer-acc-test"]
  }
}
