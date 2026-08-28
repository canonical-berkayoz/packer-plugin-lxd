# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# Details on using this Integration template can be found at https://github.com/hashicorp/integration-template
# This metadata.hcl file and the adjacent `components` docs directory should
# be kept in a `.web-docs` directory at the root of your plugin repository.
integration {
  name = "LXD"
  description = "The LXD plugin builds LXD images by talking to the LXD daemon over its API."
  identifier = "packer/canonical/lxd"
  flags = [
    # Remove if the plugin does not conform to the HCP Packer requirements.
    #
    # Please refer to our docs if you want your plugin to be compatible with
    # HCP Packer: https://developer.hashicorp.com/packer/docs/plugins/creation/hcp-support
    "hcp-ready",
  ]
  docs {
    process_docs = true
    readme_location = "./README.md"
    external_url = "https://github.com/canonical/packer-plugin-lxd"
  }
  license {
    type = "MPL-2.0"
    url = "https://github.com/canonical/packer-plugin-lxd/blob/main/LICENSE"
  }
  component {
    type = "builder"
    name = "LXD"
    slug = "lxd"
  }
}
