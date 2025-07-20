# 🚀 Packer Plugin for Meda

**Build lightning-fast VM images with the power of Cloud-Hypervisor**

Stop wrestling with slow, bloated VM creation. This Packer plugin harnesses Meda's Cloud-Hypervisor technology to spin up VMs that boot in seconds, not minutes. Whether you're building CI/CD pipelines or crafting custom images, this plugin gets you there faster.

## Why This Matters

Traditional VM creation is painfully slow. You wait. You wait some more. Then you wait for your coffee to brew while waiting for the VM to start.

Meda changes the game with:
- **⚡ Lightning startup** - VMs boot in under 3 seconds
- **🎯 Minimal overhead** - No hypervisor bloat, just what you need
- **🔧 Full control** - Every aspect of your VM is configurable
- **📦 Container-friendly** - Output images that work everywhere

## Getting Started Fast

### 1. Install the Plugin

```bash
# Grab the latest release
wget https://github.com/cirunlabs/packer-plugin-meda/releases/latest/download/packer-plugin-meda
chmod +x packer-plugin-meda
packer plugins install --path packer-plugin-meda github.com/cirunlabs/meda
```

### 2. Write Your First Template

```hcl
packer {
  required_plugins {
    meda = {
      version = ">= 1.0.0"
      source = "github.com/cirunlabs/meda"
    }
  }
}

source "meda-vm" "ubuntu" {
  vm_name           = "my-awesome-vm"
  base_image        = "ubuntu:latest"
  memory            = "2G"
  cpus              = 4
  output_image_name = "my-custom-ubuntu"

  ssh_username = "ubuntu"
}

build {
  sources = ["source.meda-vm.ubuntu"]

  provisioner "shell" {
    inline = [
      "sudo apt update && sudo apt install -y htop curl git",
      "echo 'VM built successfully!' > /tmp/success"
    ]
  }
}
```

### 3. Build It

```bash
packer build template.pkr.hcl
```

That's it. Seriously.

## Real-World Examples

### Development Environment

Perfect for creating consistent dev environments:

```hcl
source "meda-vm" "devbox" {
  vm_name     = "developer-workstation"
  base_image  = "ubuntu:22.04"
  memory      = "8G"
  cpus        = 8
  disk_size   = "100G"

  output_image_name = "devbox-2024"
  output_tag        = "latest"
}

build {
  sources = ["source.meda-vm.devbox"]

  provisioner "shell" {
    scripts = [
      "scripts/install-docker.sh",
      "scripts/setup-nodejs.sh",
      "scripts/configure-zsh.sh"
    ]
  }
}
```

### CI/CD Runner

Build stripped-down runners that start instantly:

```hcl
source "meda-vm" "ci-runner" {
  vm_name           = "ci-minimal"
  base_image        = "ubuntu:22.04"
  memory            = "4G"
  cpus              = 4
  output_image_name = "ci-runner"

  # Push to your registry
  registry     = "ghcr.io"
  organization = "yourorg"
}
```

### API Mode for Automation

When you need programmatic control:

```hcl
source "meda-vm" "automated" {
  use_api   = true
  meda_host = "build-server.internal"
  meda_port = 7777

  vm_name           = "auto-build"
  base_image        = "alpine:latest"
  output_image_name = "minimal-service"
}
```

## Configuration That Makes Sense

### Essential Settings

| Option | What It Does | Example |
|--------|-------------|---------|
| `vm_name` | Names your VM instance | `"web-server"` |
| `base_image` | Starting point image | `"ubuntu:22.04"` |
| `output_image_name` | What to call the result | `"my-app-server"` |

### Resource Control

| Option | Default | Purpose |
|--------|---------|---------|
| `memory` | `"1G"` | RAM allocation |
| `cpus` | `2` | CPU cores |
| `disk_size` | `"10G"` | Storage space |

### Meda Integration

| Option | Default | When to Use |
|--------|---------|-------------|
| `meda_binary` | `"meda"` | Custom binary path |
| `use_api` | `false` | Programmatic builds |
| `meda_host` | `"127.0.0.1"` | Remote Meda server |
| `meda_port` | `7777` | Custom API port |

## Pro Tips

**🔥 Speed up builds**: Use `user_data_file` for cloud-init instead of shell provisioners for system setup.

**🛡️ Security first**: The plugin automatically generates unique VM names to prevent conflicts.

**📊 Debug like a pro**: Set `PACKER_LOG=1` to see exactly what's happening under the hood.

**🎯 Template variables**: Use `{{ .MedaVMName }}` and `{{ .MedaVMIP }}` in your provisioner scripts.

## Troubleshooting the Usual Suspects

**"Meda binary not found"**
→ Install Meda or specify the full path with `meda_binary`

**"VM creation timeout"**
→ Bump up `ssh_timeout` or check if your base image supports SSH

**"API connection failed"**
→ Make sure Meda server is running: `meda serve`

## What's Under the Hood

This plugin is built with the official Packer SDK and leverages Meda's Cloud-Hypervisor backend for maximum performance. It handles the complete VM lifecycle - create, provision, snapshot, cleanup - so you don't have to think about it.

The source code is clean, well-tested, and follows Packer plugin best practices. We use GitHub Actions for CI/CD and maintain backward compatibility.

## Contributing

Found a bug? Have an idea? We're all ears:

1. **Fork it** - Grab your own copy
2. **Branch it** - `git checkout -b feature/amazing-feature`
3. **Code it** - Make your changes
4. **Test it** - Ensure everything works
5. **Ship it** - Submit a pull request

## Releasing

To create a new release of the plugin:

### 1. Update Version

First, update the version in the plugin code:

```bash
cd plugin
# Update version in main.go if needed
vim main.go
```

### 2. Create and Push Tag

```bash
# Create a new tag (replace with actual version)
git tag v1.0.0
git push origin v1.0.0
```

### 3. Automated Release

The release workflow will automatically:
- Build binaries for all supported platforms (Linux, macOS, Windows)
- Generate checksums
- Create a GitHub release with release notes
- Upload all artifacts

### 4. Manual Steps (if needed)

If something goes wrong with the automated release:

```bash
# Install GoReleaser locally
go install github.com/goreleaser/goreleaser@latest

# Test the release (dry run)
cd plugin
goreleaser release --snapshot --clean

# Create actual release (requires GITHUB_TOKEN)
export GITHUB_TOKEN=your_token_here
goreleaser release --clean
```

### 5. Verify Release

After release:
1. Check the [releases page](https://github.com/cirunlabs/packer-plugin-meda/releases)
2. Download and test binaries
3. Verify checksums match
4. Test installation instructions

### Release Checklist

- [ ] All tests pass
- [ ] Documentation is up to date
- [ ] Version is bumped
- [ ] Tag is created and pushed
- [ ] GitHub Actions workflow completes successfully
- [ ] Release artifacts are uploaded
- [ ] Installation instructions work

## License

This project is licensed under the Mozilla Public License Version 2.0 - see the [LICENSE](LICENSE) file for details.

---

**Built by developers, for developers.**

Need help? Open an issue. Want to chat? Find us in the discussions. Building something cool? We'd love to hear about it.