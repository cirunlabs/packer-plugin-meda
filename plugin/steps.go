package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"
)

// getMedaDir returns the dynamic path to the meda directory
func getMedaDir() (string, error) {
	currentUser, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("failed to get current user: %v", err)
	}
	return filepath.Join(currentUser.HomeDir, "meda"), nil
}

// stepCreateBaseImage ensures the base image is available locally by creating it
type stepCreateBaseImage struct{}

func (s *stepCreateBaseImage) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	config := state.Get("config").(*Config)
	ui := state.Get("ui").(packer.Ui)

	// Extract base image name without tag (e.g., "ubuntu-base:latest" -> "ubuntu-base")
	baseImageName := config.BaseImage
	if strings.Contains(baseImageName, ":") {
		baseImageName = strings.Split(baseImageName, ":")[0]
	}

	ui.Say("Ensuring base image '" + config.BaseImage + "' is available locally")

	// First check if image exists locally
	var checkCmd *exec.Cmd
	if config.UseAPI {
		checkCmd = exec.Command("curl", "-s",
			fmt.Sprintf("http://%s:%d/api/v1/images", config.MedaHost, config.MedaPort))
	} else {
		if config.MedaBinary == "cargo" {
			medaDir, err := getMedaDir()
			if err != nil {
				err := fmt.Errorf("failed to get meda directory: %s", err)
				state.Put("error", err)
				ui.Error(err.Error())
				return multistep.ActionHalt
			}
			checkCmd = exec.Command("cargo", "run", "--", "images")
			checkCmd.Dir = medaDir
		} else {
			checkCmd = exec.Command(config.MedaBinary, "images")
		}
	}

	output, err := checkCmd.CombinedOutput()
	imageExists := err == nil && strings.Contains(string(output), baseImageName)

	if !imageExists {
		// For ubuntu-base, create from ubuntu base. For ubuntu, create basic ubuntu image
		if baseImageName == "ubuntu-base" {
			ui.Say("Base image 'ubuntu-base' not found locally, creating from ubuntu...")
			// First ensure ubuntu base image exists
			if err := s.ensureUbuntuBaseImage(config, ui); err != nil {
				state.Put("error", err)
				ui.Error(err.Error())
				return multistep.ActionHalt
			}
		} else {
			ui.Say("Base image '" + baseImageName + "' not found locally, creating basic Ubuntu image...")
		}

		var createCmd *exec.Cmd
		if config.UseAPI {
			// Use API to create image
			createCmd = exec.Command("curl", "-X", "POST",
				fmt.Sprintf("http://%s:%d/api/v1/images", config.MedaHost, config.MedaPort),
				"-H", "Content-Type: application/json",
				"-d", fmt.Sprintf(`{
					"name": "%s",
					"tag": "latest"
				}`, baseImageName))
		} else {
			if config.MedaBinary == "cargo" {
				medaDir, err := getMedaDir()
				if err != nil {
					err := fmt.Errorf("failed to get meda directory: %s", err)
					state.Put("error", err)
					ui.Error(err.Error())
					return multistep.ActionHalt
				}
				createCmd = exec.Command("cargo", "run", "--", "create-image", baseImageName)
				createCmd.Dir = medaDir
			} else {
				createCmd = exec.Command(config.MedaBinary, "create-image", baseImageName)
			}
		}

		// Create pipes to capture and display output
		stdout, err := createCmd.StdoutPipe()
		if err != nil {
			return multistep.ActionHalt
		}
		stderr, err := createCmd.StderrPipe()
		if err != nil {
			return multistep.ActionHalt
		}

		// Start the command
		if err := createCmd.Start(); err != nil {
			err := fmt.Errorf("failed to start create-image command: %s", err)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}

		// Read and display output in real-time
		var stderrOutput strings.Builder

		// Handle stdout
		go func() {
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				ui.Say(scanner.Text())
			}
		}()

		// Handle stderr and capture it for error checking
		go func() {
			stderrScanner := bufio.NewScanner(stderr)
			for stderrScanner.Scan() {
				line := stderrScanner.Text()
				stderrOutput.WriteString(line + "\n")
				ui.Say(line)
			}
		}()

		// Wait for command to finish
		createErr := createCmd.Wait()

		// Give goroutines a moment to finish reading
		time.Sleep(100 * time.Millisecond)

		// Check for errors
		stderrContent := stderrOutput.String()
		if createErr != nil {
			errorMsg := "failed to create base image '" + baseImageName + "'"
			if createErr != nil {
				errorMsg += ": " + createErr.Error()
			}
			if stderrContent != "" {
				errorMsg += " - " + strings.TrimSpace(stderrContent)
			}

			err := fmt.Errorf("%s", errorMsg)
			state.Put("error", err)
			ui.Error(errorMsg)
			return multistep.ActionHalt
		}

		ui.Say("Successfully created base image '" + baseImageName + "'")
	} else {
		ui.Say("Base image '" + baseImageName + "' already available locally")
	}

	return multistep.ActionContinue
}

// ensureUbuntuBaseImage creates the ubuntu base image if it doesn't exist
func (s *stepCreateBaseImage) ensureUbuntuBaseImage(config *Config, ui packer.Ui) error {
	// Check if ubuntu image exists
	var checkCmd *exec.Cmd
	if config.MedaBinary == "cargo" {
		medaDir, err := getMedaDir()
		if err != nil {
			return fmt.Errorf("failed to get meda directory: %s", err)
		}
		checkCmd = exec.Command("cargo", "run", "--", "images")
		checkCmd.Dir = medaDir
	} else {
		checkCmd = exec.Command(config.MedaBinary, "images")
	}

	output, err := checkCmd.CombinedOutput()
	ubuntuExists := err == nil && strings.Contains(string(output), "ubuntu")

	if !ubuntuExists {
		ui.Say("Creating basic Ubuntu image first...")

		var createCmd *exec.Cmd
		if config.MedaBinary == "cargo" {
			medaDir, err := getMedaDir()
			if err != nil {
				return fmt.Errorf("failed to get meda directory: %s", err)
			}
			createCmd = exec.Command("cargo", "run", "--", "create-image", "ubuntu")
			createCmd.Dir = medaDir
		} else {
			createCmd = exec.Command(config.MedaBinary, "create-image", "ubuntu")
		}

		output, err := createCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to create ubuntu base image: %s - %s", err, string(output))
		}

		ui.Say("Successfully created basic Ubuntu image")
	}

	return nil
}

func (s *stepCreateBaseImage) Cleanup(state multistep.StateBag) {
	// No cleanup needed for image creation
}

// stepCreateVM creates a new VM using Meda
type stepCreateVM struct{}

func (s *stepCreateVM) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	config := state.Get("config").(*Config)
	ui := state.Get("ui").(packer.Ui)
	vmName := state.Get("vm_name").(string)

	ui.Say("Creating VM '" + vmName + "' with base image '" + config.BaseImage + "'")

	var cmd *exec.Cmd
	if config.UseAPI {
		// Use REST API to create VM
		cmd = exec.Command("curl", "-X", "POST",
			fmt.Sprintf("http://%s:%d/api/v1/vms", config.MedaHost, config.MedaPort),
			"-H", "Content-Type: application/json",
			"-d", fmt.Sprintf(`{
				"name": "%s",
				"base_image": "%s",
				"memory": "%s",
				"cpus": %d,
				"disk": "%s",
				"force": false
			}`, vmName, config.BaseImage, config.Memory, config.CPUs, config.DiskSize))
	} else {
		// Use CLI to create VM
		args := []string{"run", config.BaseImage, "--name", vmName,
			"--memory", config.Memory,
			"--cpus", fmt.Sprintf("%d", config.CPUs),
			"--disk", config.DiskSize,
			"--no-start"}

		if config.UserDataFile != "" {
			args = append(args, "--user-data", config.UserDataFile)
		}

		// Use cargo run for development
		if config.MedaBinary == "cargo" {
			medaDir, err := getMedaDir()
			if err != nil {
				err := fmt.Errorf("failed to get meda directory: %s", err)
				state.Put("error", err)
				ui.Error(err.Error())
				return multistep.ActionHalt
			}
			cargoArgs := append([]string{"run", "--"}, args...)
			cmd = exec.Command("cargo", cargoArgs...)
			cmd.Dir = medaDir // Set working directory for cargo
		} else {
			cmd = exec.Command(config.MedaBinary, args...)
		}
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		err := fmt.Errorf("failed to create VM: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	ui.Say("VM '" + vmName + "' created successfully")
	return multistep.ActionContinue
}

func (s *stepCreateVM) Cleanup(state multistep.StateBag) {
	// Cleanup will be handled by stepCleanupVM
}

// stepStartVM starts the VM
type stepStartVM struct{}

func (s *stepStartVM) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	config := state.Get("config").(*Config)
	ui := state.Get("ui").(packer.Ui)
	vmName := state.Get("vm_name").(string)

	ui.Say("Starting VM '" + vmName + "'")

	var cmd *exec.Cmd
	if config.UseAPI {
		cmd = exec.Command("curl", "-X", "POST",
			fmt.Sprintf("http://%s:%d/api/v1/vms/%s/start", config.MedaHost, config.MedaPort, vmName))
	} else {
		if config.MedaBinary == "cargo" {
			medaDir, err := getMedaDir()
			if err != nil {
				err := fmt.Errorf("failed to get meda directory: %s", err)
				state.Put("error", err)
				ui.Error(err.Error())
				return multistep.ActionHalt
			}
			cmd = exec.Command("cargo", "run", "--", "start", vmName)
			cmd.Dir = medaDir
		} else {
			cmd = exec.Command(config.MedaBinary, "start", vmName)
		}
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		err := fmt.Errorf("failed to start VM: %s - %s", err, string(output))
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	ui.Say("VM '" + vmName + "' started successfully")
	return multistep.ActionContinue
}

func (s *stepStartVM) Cleanup(state multistep.StateBag) {}

// stepWaitForVM waits for the VM to be ready and gets its IP
type stepWaitForVM struct{}

func (s *stepWaitForVM) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	config := state.Get("config").(*Config)
	ui := state.Get("ui").(packer.Ui)
	vmName := state.Get("vm_name").(string)

	ui.Say("Waiting for VM '" + vmName + "' to be ready...")

	// Wait for VM to be running and get IP
	timeout := time.After(5 * time.Minute)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			err := fmt.Errorf("timeout waiting for VM to be ready")
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		case <-ticker.C:
			var cmd *exec.Cmd
			if config.UseAPI {
				cmd = exec.Command("curl", "-s",
					fmt.Sprintf("http://%s:%d/api/v1/vms/%s/ip", config.MedaHost, config.MedaPort, vmName))
			} else {
				if config.MedaBinary == "cargo" {
					medaDir, err := getMedaDir()
					if err != nil {
						// Just log and return error for this specific case
						ui.Error("failed to get meda directory: " + err.Error())
						return multistep.ActionHalt
					}
					cmd = exec.Command("cargo", "run", "--", "ip", vmName)
					cmd.Dir = medaDir
				} else {
					cmd = exec.Command(config.MedaBinary, "ip", vmName)
				}
			}

			output, err := cmd.CombinedOutput()
			if err == nil && len(output) > 0 {
				// Extract only the IP address from the output
				// The output might contain cargo build information
				lines := strings.Split(string(output), "\n")
				var ip string
				for _, line := range lines {
					line = strings.TrimSpace(line)
					// Check if this line looks like an IP address
					if strings.Count(line, ".") == 3 && !strings.Contains(line, " ") {
						// Basic IP validation
						parts := strings.Split(line, ".")
						if len(parts) == 4 {
							valid := true
							for _, part := range parts {
								if _, err := strconv.Atoi(part); err != nil {
									valid = false
									break
								}
							}
							if valid {
								ip = line
								break
							}
						}
					}
				}

				if ip != "" && ip != "null" {
					state.Put("vm_ip", ip)
					state.Put("instance_ip", ip)
					// Set SSH host in the communicator config
					config.Comm.SSHHost = ip
					ui.Say("VM is ready with IP: " + ip)
					return multistep.ActionContinue
				}
			}
			ui.Say("VM not ready yet, waiting...")
		}
	}
}

func (s *stepWaitForVM) Cleanup(state multistep.StateBag) {}

// stepStopVM stops the VM
type stepStopVM struct{}

func (s *stepStopVM) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	config := state.Get("config").(*Config)
	ui := state.Get("ui").(packer.Ui)
	vmName := state.Get("vm_name").(string)

	ui.Say("Stopping VM '" + vmName + "'")

	var cmd *exec.Cmd
	if config.UseAPI {
		cmd = exec.Command("curl", "-X", "POST",
			fmt.Sprintf("http://%s:%d/api/v1/vms/%s/stop", config.MedaHost, config.MedaPort, vmName))
	} else {
		if config.MedaBinary == "cargo" {
			cmd = exec.Command("cargo", "run", "--", "stop", vmName)
			medaDir, err := getMedaDir()
			if err != nil {
				return multistep.ActionHalt
			}
			cmd.Dir = medaDir
		} else {
			cmd = exec.Command(config.MedaBinary, "stop", vmName)
		}
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Warning: failed to stop VM: %s - %s", err, string(output))
		// Continue anyway - VM might already be stopped
	} else {
		ui.Say("VM '" + vmName + "' stopped successfully")
	}

	return multistep.ActionContinue
}

func (s *stepStopVM) Cleanup(state multistep.StateBag) {}

// stepCreateImage creates an image from the VM
type stepCreateImage struct{}

func (s *stepCreateImage) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	config := state.Get("config").(*Config)
	ui := state.Get("ui").(packer.Ui)
	vmName := state.Get("vm_name").(string)

	imageName := fmt.Sprintf("%s:%s", config.OutputImageName, config.OutputTag)
	ui.Say("Creating image '" + imageName + "' from VM '" + vmName + "'")

	var cmd *exec.Cmd
	if config.UseAPI {
		cmd = exec.Command("curl", "-X", "POST",
			fmt.Sprintf("http://%s:%d/api/v1/images", config.MedaHost, config.MedaPort),
			"-H", "Content-Type: application/json",
			"-d", fmt.Sprintf(`{
				"name": "%s",
				"tag": "%s",
				"from_vm": "%s"
			}`, config.OutputImageName, config.OutputTag, vmName))
	} else {
		if config.MedaBinary == "cargo" {
			cmd = exec.Command("cargo", "run", "--", "create-image", config.OutputImageName,
				"--tag", config.OutputTag,
				"--from-vm", vmName)
			medaDir, err := getMedaDir()
			if err != nil {
				return multistep.ActionHalt
			}
			cmd.Dir = medaDir
		} else {
			cmd = exec.Command(config.MedaBinary, "create-image", config.OutputImageName,
				"--tag", config.OutputTag,
				"--from-vm", vmName)
		}
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		err := fmt.Errorf("failed to create image: %s - %s", err, string(output))
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	state.Put("image_name", imageName)
	ui.Say("Image '" + imageName + "' created successfully")
	return multistep.ActionContinue
}

func (s *stepCreateImage) Cleanup(state multistep.StateBag) {}

// stepPushImage pushes the created image to a registry
type stepPushImage struct{}

func (s *stepPushImage) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	config := state.Get("config").(*Config)
	ui := state.Get("ui").(packer.Ui)
	imageName := state.Get("image_name").(string)

	// Skip push if not enabled
	if !config.PushToRegistry {
		ui.Say("Push to registry disabled, skipping push step")
		return multistep.ActionContinue
	}

	// Check for GITHUB_TOKEN when pushing to GHCR
	if strings.Contains(config.Registry, "ghcr.io") {
		if os.Getenv("GITHUB_TOKEN") == "" {
			err := fmt.Errorf("GITHUB_TOKEN environment variable is required for pushing to GHCR. Please set it with: export GITHUB_TOKEN=your_token")
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}
		ui.Say("GITHUB_TOKEN found for GHCR authentication")
	}

	// Build target image name
	var targetImage string
	if config.Organization != "" {
		targetImage = fmt.Sprintf("%s/%s/%s:%s", config.Registry, config.Organization, config.OutputImageName, config.OutputTag)
	} else {
		targetImage = fmt.Sprintf("%s/%s:%s", config.Registry, config.OutputImageName, config.OutputTag)
	}

	ui.Say("Pushing image '" + imageName + "' to '" + targetImage + "'")

	var cmd *exec.Cmd
	if config.UseAPI {
		// Use REST API to push image
		pushData := fmt.Sprintf(`{
			"name": "%s",
			"image": "%s",
			"registry": "%s",
			"dry_run": %t
		}`, imageName, targetImage, config.Registry, config.DryRun)

		cmd = exec.Command("curl", "-X", "POST",
			fmt.Sprintf("http://%s:%d/api/v1/images/push", config.MedaHost, config.MedaPort),
			"-H", "Content-Type: application/json",
			"-d", pushData)
	} else {
		// Use CLI to push image - Meda expects just the image name without tag
		imageNameOnly := config.OutputImageName
		args := []string{"push", imageNameOnly, targetImage}
		if config.Registry != "" && config.Registry != "ghcr.io" {
			args = append(args, "--registry", config.Registry)
		}
		if config.DryRun {
			args = append(args, "--dry-run")
		}

		if config.MedaBinary == "cargo" {
			cargoArgs := append([]string{"run", "--"}, args...)
			cmd = exec.Command("cargo", cargoArgs...)
			medaDir, err := getMedaDir()
			if err != nil {
				return multistep.ActionHalt
			}
			cmd.Dir = medaDir
		} else {
			cmd = exec.Command(config.MedaBinary, args...)
		}
	}

	// Create pipes to capture and display output
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return multistep.ActionHalt
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return multistep.ActionHalt
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		err := fmt.Errorf("failed to start push command: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	// Read and display output in real-time
	var stderrOutput strings.Builder

	// Handle stdout
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			ui.Say(scanner.Text())
		}
	}()

	// Handle stderr and capture it for error checking
	go func() {
		stderrScanner := bufio.NewScanner(stderr)
		for stderrScanner.Scan() {
			line := stderrScanner.Text()
			stderrOutput.WriteString(line + "\n")
			ui.Say(line)
		}
	}()

	// Wait for command to finish
	pushErr := cmd.Wait()

	// Give goroutines a moment to finish reading
	time.Sleep(100 * time.Millisecond)

	// Check for errors in stderr content
	stderrContent := stderrOutput.String()
	if pushErr != nil || strings.Contains(stderrContent, "unauthorized") || strings.Contains(stderrContent, "denied") || strings.Contains(stderrContent, "authentication required") {
		errorMsg := "failed to push image"
		if pushErr != nil {
			errorMsg += ": " + pushErr.Error()
		}
		if stderrContent != "" {
			errorMsg += " - " + strings.TrimSpace(stderrContent)
		}
		err := fmt.Errorf("%s", errorMsg)
		state.Put("error", err)
		ui.Error(errorMsg)
		return multistep.ActionHalt
	}

	ui.Say("Image '" + imageName + "' pushed successfully to '" + targetImage + "'")
	state.Put("pushed_image", targetImage)
	return multistep.ActionContinue
}

func (s *stepPushImage) Cleanup(state multistep.StateBag) {}

// stepCleanupVM cleans up the VM
type stepCleanupVM struct{}

func (s *stepCleanupVM) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	config := state.Get("config").(*Config)
	ui := state.Get("ui").(packer.Ui)
	vmName := state.Get("vm_name").(string)

	ui.Say("Cleaning up VM '" + vmName + "'")

	var cmd *exec.Cmd
	if config.UseAPI {
		cmd = exec.Command("curl", "-X", "DELETE",
			fmt.Sprintf("http://%s:%d/api/v1/vms/%s", config.MedaHost, config.MedaPort, vmName))
	} else {
		if config.MedaBinary == "cargo" {
			cmd = exec.Command("cargo", "run", "--", "delete", vmName)
			medaDir, err := getMedaDir()
			if err != nil {
				return multistep.ActionHalt
			}
			cmd.Dir = medaDir
		} else {
			cmd = exec.Command(config.MedaBinary, "delete", vmName)
		}
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Warning: failed to delete VM: %s - %s", err, string(output))
		// Continue anyway - cleanup is best effort
	} else {
		ui.Say("VM '" + vmName + "' cleaned up successfully")
	}

	return multistep.ActionContinue
}

func (s *stepCleanupVM) Cleanup(state multistep.StateBag) {
	// This is the cleanup step itself
}

