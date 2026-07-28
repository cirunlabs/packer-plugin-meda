variable "image_tag" {
  type        = string
  default     = "latest"
  description = "Tag for the output image"
}

variable "registry" {
  type        = string
  default     = "ghcr.io"
  description = "Container registry to push to"
}

variable "organization" {
  type        = string
  default     = env("GITHUB_REPOSITORY_OWNER") != "" ? env("GITHUB_REPOSITORY_OWNER") : "cirunlabs"
  description = "Registry organization/namespace"
}

variable "push_enabled" {
  type        = bool
  default     = true
  description = "Whether to push the image to registry"
}

variable "dry_run" {
  type        = bool
  default     = false
  description = "Dry run mode"
}

variable "meda_binary_path" {
  type        = string
  default     = env("MEDA_BINARY_PATH") != "" ? env("MEDA_BINARY_PATH") : "meda"
  description = "Path to the meda binary to use"
}

# NVIDIA server (datacenter) driver branch. Pinned for reproducibility.
# 580-server supports Ada-generation datacenter GPUs (e.g. RTX 4000 SFF Ada,
# 10de:27b0). Bump deliberately, never floating.
variable "nvidia_driver_branch" {
  type        = string
  default     = "580"
  description = "NVIDIA -server driver branch to install"
}

source "meda-vm" "ubuntu-gpu" {
  vm_name    = "ubuntu-gpu-build"
  base_image = "ubuntu:latest"
  memory     = "4G"
  cpus       = 4
  # NVIDIA driver + built kernel modules need headroom over the slim base.
  disk_size = "12G"

  output_image_name = "ubuntu-gpu"
  output_tag        = var.image_tag
  registry          = var.registry
  organization      = var.organization

  push_to_registry = var.push_enabled
  dry_run          = var.dry_run

  meda_binary = var.meda_binary_path

  ssh_username = "cirun"
  ssh_password = "cirun"
  ssh_timeout  = "3m"
  ssh_port     = 22
}

build {
  name    = "ubuntu-gpu"
  sources = ["source.meda-vm.ubuntu-gpu"]

  # Wait for cloud-init to complete
  provisioner "shell" {
    inline = [
      "echo 'Waiting for cloud-init to complete...'",
      "cloud-init status --wait || true",
      "echo 'Cloud-init completed'"
    ]
  }

  # Runtime deps meda itself needs to create VMs (genisoimage for the
  # cloud-init ISO, qemu-img from qemu-utils for qcow2 backing files).
  # Neither ships in the base ubuntu cloud image. Without them, meda
  # running inside this VM (nested, on top of the host's own meda VM)
  # fails before it ever reaches cloud-hypervisor.
  provisioner "shell" {
    inline = [
      "echo 'Installing meda runtime dependencies...'",
      "sudo apt-get update",
      "sudo DEBIAN_FRONTEND=noninteractive apt-get install -y genisoimage qemu-utils"
    ]
  }

  # Blacklist nouveau so the proprietary NVIDIA driver can bind the card.
  # Must be in the initramfs, hence update-initramfs after writing the conf.
  provisioner "shell" {
    inline = [
      "echo 'Blacklisting nouveau...'",
      "printf 'blacklist nouveau\\noptions nouveau modeset=0\\n' | sudo tee /etc/modprobe.d/blacklist-nouveau.conf",
      "sudo update-initramfs -u"
    ]
  }

  # Install the pinned NVIDIA -server driver + userspace (nvidia-smi lives in
  # nvidia-utils). Headers for the running guest kernel guarantee the kernel
  # module builds/matches. -server branch is the headless/datacenter variant.
  provisioner "shell" {
    inline = [
      "echo 'Installing NVIDIA ${var.nvidia_driver_branch}-server driver...'",
      "sudo apt-get update",
      "sudo DEBIAN_FRONTEND=noninteractive apt-get install -y linux-headers-$(uname -r) build-essential",
      "sudo DEBIAN_FRONTEND=noninteractive apt-get install -y nvidia-driver-${var.nvidia_driver_branch}-server nvidia-utils-${var.nvidia_driver_branch}-server",
      "dpkg -l | grep -i nvidia-driver || true"
    ]
  }

  # Load the NVIDIA modules at boot (headless: no X, no display manager).
  provisioner "shell" {
    inline = [
      "echo 'Configuring boot-time module load + persistence...'",
      "printf 'nvidia\\nnvidia_modeset\\nnvidia_uvm\\n' | sudo tee /etc/modules-load.d/nvidia.conf",
      "sudo systemctl enable nvidia-persistenced || true"
    ]
  }

  # Verify the module built for THIS guest kernel. nvidia-smi cannot run at
  # build time (no GPU on the build VM), so we assert the module + userspace
  # are present; the runtime nvidia-smi check happens on a passthrough host.
  provisioner "shell" {
    inline = [
      "echo 'Verifying driver artifacts...'",
      "test -f /usr/bin/nvidia-smi && echo 'nvidia-smi present'",
      "modinfo nvidia | head -3",
      "echo 'NVIDIA driver baked in'"
    ]
  }

  # Cleanup (keep the nvidia modules + dkms trees; only drop caches/logs).
  provisioner "shell" {
    inline = [
      "echo 'Cleaning up for image creation...'",
      "sudo apt-get autoremove -y",
      "sudo apt-get autoclean",
      "sudo apt-get clean",
      "sudo rm -rf /var/lib/apt/lists/*",
      "sudo rm -rf /tmp/* /var/tmp/*",
      "sudo rm -rf /var/cache/apt/archives/*",
      "sudo rm -rf /home/cirun/.cache/*",
      "sudo rm -rf /root/.cache/*",
      "sudo find /var/log -type f -exec truncate -s 0 {} \\;",
      "sudo rm -rf /var/log/journal/*",
      "echo 'Image preparation completed'"
    ]
  }

  post-processor "manifest" {
    output      = "manifest.json"
    strip_path  = true
    custom_data = {
      image_name = "ubuntu-gpu"
      image_tag  = var.image_tag
      build_time = timestamp()
      vm_name    = "{{ .MedaVMName }}"
      vm_ip      = "{{ .MedaVMIP }}"
    }
  }
}
