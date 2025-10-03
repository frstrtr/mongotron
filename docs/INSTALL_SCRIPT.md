# Prerequisites Installation Script

## Overview

The `install-prerequisites.sh` script automates the installation of all required tools and dependencies for developing and running MongoTron. It handles system updates, Go installation, Docker setup, development tools, and project dependencies.

## Features

✅ **Automated Installation**
- System package updates
- Go programming language (via snap)
- Docker and Docker Compose
- Protocol Buffers compiler (protoc)
- Go development tools (golangci-lint, goimports, etc.)
- Project dependencies

✅ **Smart Detection**
- Checks for existing installations
- Version validation
- OS compatibility checking
- Graceful handling of partial installations

✅ **Configurable**
- Skip options for individual components
- Verbose mode for debugging
- Detailed logging to file
- Color-coded terminal output

✅ **Safe & Robust**
- Root user prevention
- Error handling with `set -e`
- Sudo privilege management
- Comprehensive verification

## Quick Start

### Basic Installation (Everything)

```bash
# Run from the MongoTron project directory
./scripts/install-prerequisites.sh
```

This will install:
- Go 1.24.6
- Docker 27.5.1 & Docker Compose 1.29.2
- Protocol Buffers compiler
- golangci-lint v1.55.2
- goimports, protoc-gen-go, protoc-gen-go-grpc
- All Go module dependencies
- Optional development tools (jq, tree, htop, etc.)

### Installation with Options

```bash
# Skip Docker installation (if already installed)
./scripts/install-prerequisites.sh --skip-docker

# Skip Go installation (if already installed)
./scripts/install-prerequisites.sh --skip-go

# Skip development tools
./scripts/install-prerequisites.sh --skip-tools

# Skip project dependencies download
./scripts/install-prerequisites.sh --skip-deps

# Verbose mode for debugging
./scripts/install-prerequisites.sh --verbose

# Combine multiple options
./scripts/install-prerequisites.sh --skip-docker --skip-deps
```

## Usage

```
Usage: ./scripts/install-prerequisites.sh [OPTIONS]

Install all prerequisites for MongoTron development.

OPTIONS:
    -h, --help              Show this help message
    --skip-docker           Skip Docker and Docker Compose installation
    --skip-go               Skip Go installation
    --skip-tools            Skip Go development tools installation
    --skip-deps             Skip project dependencies download
    --verbose               Enable verbose output
    -y, --yes               Skip all confirmations

EXAMPLES:
    ./scripts/install-prerequisites.sh                    # Install everything
    ./scripts/install-prerequisites.sh --skip-docker      # Install without Docker
    ./scripts/install-prerequisites.sh --skip-deps        # Install without downloading dependencies
    ./scripts/install-prerequisites.sh -y                 # Install everything without confirmations
```

## What Gets Installed

### Core Tools

| Component | Version | Method | Purpose |
|-----------|---------|--------|---------|
| Go | 1.24.6 | snap | Primary programming language |
| Docker | 27.5.1 | apt | Container runtime |
| Docker Compose | 1.29.2 | apt | Multi-container orchestration |
| Protocol Buffers | 3.21.12 | apt | gRPC code generation |

### Go Development Tools

| Tool | Installation | Purpose |
|------|--------------|---------|
| golangci-lint | `go install` v1.55.2 | Code linting and static analysis |
| goimports | `go install` latest | Import formatting and organization |
| protoc-gen-go | `go install` latest | Protobuf to Go code generation |
| protoc-gen-go-grpc | `go install` latest | gRPC service code generation |
| govulncheck | `go install` latest | Vulnerability scanning |

### Optional Utilities

- `jq` - JSON processing
- `tree` - Directory visualization
- `htop` - Process monitoring
- `net-tools` - Network utilities
- `vim` - Text editor

### Project Dependencies

All Go modules defined in `go.mod`:
- MongoDB driver
- gRPC and Protobuf libraries
- Gorilla WebSocket
- Viper configuration
- Zerolog logging
- Prometheus metrics
- And 30+ other dependencies

## Installation Process

The script follows this sequence:

1. **Preflight Checks**
   - Verify not running as root
   - Detect OS (Ubuntu/Debian)
   - Request sudo privileges if needed

2. **System Update**
   - Update apt package lists
   - Install essential build tools
   - Install certificates and dependencies

3. **Go Installation**
   - Check existing Go version
   - Install via snap if needed
   - Configure GOPATH and PATH
   - Create Go workspace directories

4. **Go Tools Installation**
   - Install golangci-lint
   - Install goimports
   - Install protoc-gen-go and protoc-gen-go-grpc
   - Install additional development tools

5. **Protocol Buffers**
   - Install protobuf-compiler package
   - Verify protoc installation

6. **Docker Installation**
   - Install docker.io package
   - Start and enable Docker service
   - Add user to docker group

7. **Docker Compose Installation**
   - Install docker-compose package
   - Verify installation

8. **Project Dependencies**
   - Download Go modules (`go mod download`)
   - Tidy modules (`go mod tidy`)

9. **Optional Tools**
   - Install development utilities

10. **Verification**
    - Check all installations
    - Display component versions
    - Report any issues

## Post-Installation Steps

After running the script, you need to complete these steps:

### 1. Reload Shell Configuration

```bash
# Option A: Source your bashrc
source ~/.bashrc

# Option B: Start a new terminal session
# Close and reopen your terminal
```

This ensures Go tools are in your PATH.

### 2. Apply Docker Group Membership

```bash
# Option A: Use newgrp (temporary for current shell)
newgrp docker

# Option B: Logout and login again (permanent)
# This is the recommended approach
```

This allows you to use Docker without sudo.

### 3. Verify Installation

```bash
# Check Go
go version

# Check Go tools
which golangci-lint
golangci-lint --version

# Check Docker (should work without sudo after step 2)
docker --version
docker ps

# Check protoc
protoc --version
```

### 4. Build MongoTron

```bash
# Verify dependencies
make deps

# Build all binaries
make build

# Run tests
make test

# Start the application
make run
```

## Logging

All installation output is logged to `scripts/install.log` in the project directory.

### Log Format

```
[2025-10-03 10:30:45] Installing Go 1.24.6 via snap...
[SUCCESS] Go installed successfully: go1.24.6
[INFO] Configuring Go environment...
[SUCCESS] Go environment configured
```

### Viewing Logs

```bash
# View entire log
cat scripts/install.log

# Follow log during installation
tail -f scripts/install.log

# Search for errors
grep ERROR scripts/install.log

# Search for warnings
grep WARNING scripts/install.log
```

## Troubleshooting

### Issue: "Command not found" for Go tools

**Solution:**
```bash
# Add Go bin directory to PATH
export PATH=$PATH:$HOME/go/bin

# Make it permanent
echo 'export PATH=$PATH:$HOME/go/bin' >> ~/.bashrc
source ~/.bashrc
```

### Issue: Docker permission denied

**Solution:**
```bash
# Verify you're in docker group
groups | grep docker

# If not, run
sudo usermod -aG docker $USER

# Then logout/login or run
newgrp docker
```

### Issue: Go version too old

**Solution:**
```bash
# Remove old Go installation
sudo snap remove go

# Re-run script
./scripts/install-prerequisites.sh --skip-docker --skip-tools --skip-deps
```

### Issue: protoc not found after installation

**Solution:**
```bash
# Reinstall protobuf-compiler
sudo apt update
sudo apt install --reinstall protobuf-compiler

# Verify
protoc --version
```

### Issue: go mod download fails

**Solution:**
```bash
# Clean module cache
go clean -modcache

# Try again
go mod download
go mod tidy
```

## OS Compatibility

### Supported Operating Systems

✅ **Officially Supported:**
- Ubuntu 20.04 LTS
- Ubuntu 22.04 LTS
- Ubuntu 24.04 LTS
- Debian 11 (Bullseye)
- Debian 12 (Bookworm)

⚠️ **May Work (with warnings):**
- Other Debian-based distributions
- Pop!_OS
- Linux Mint

❌ **Not Supported:**
- RHEL/CentOS/Fedora (different package manager)
- Arch Linux
- macOS
- Windows (use WSL2)

### For Unsupported OS

If you're on a non-Ubuntu/Debian system, you can:

1. **Use the script as a reference** - Follow the installation steps manually
2. **Adapt for your package manager** - Replace `apt` with `yum`, `pacman`, etc.
3. **Install manually** - See PREREQUISITES_INSTALLED.md for manual steps

## Advanced Usage

### Running as Part of CI/CD

```yaml
# Example GitHub Actions workflow
- name: Install Prerequisites
  run: |
    chmod +x scripts/install-prerequisites.sh
    ./scripts/install-prerequisites.sh -y --skip-docker
```

### Docker-Based Development

If you prefer Docker-based development and don't need local tools:

```bash
# Skip local installation entirely
./scripts/install-prerequisites.sh --skip-go --skip-tools --skip-deps

# Only install Docker
sudo apt update && sudo apt install -y docker.io docker-compose
```

### Selective Installation

```bash
# Only Go and tools (no Docker)
./scripts/install-prerequisites.sh --skip-docker --skip-deps

# Only Docker (have Go already)
./scripts/install-prerequisites.sh --skip-go --skip-tools --skip-deps

# Minimal (no optional tools)
# Edit script and comment out install_optional_tools call
```

## Script Architecture

### Functions Overview

| Function | Purpose |
|----------|---------|
| `check_os()` | Detect and validate operating system |
| `check_root()` | Prevent running as root |
| `check_sudo()` | Ensure sudo privileges available |
| `update_system()` | Update package lists and install essentials |
| `install_go()` | Install Go programming language |
| `setup_go_env()` | Configure GOPATH and PATH |
| `install_go_tools()` | Install development tools |
| `install_protoc()` | Install Protocol Buffers compiler |
| `install_docker()` | Install Docker runtime |
| `install_docker_compose()` | Install Docker Compose |
| `install_project_deps()` | Download Go module dependencies |
| `install_optional_tools()` | Install additional utilities |
| `verify_installation()` | Check all installations succeeded |
| `print_post_install_info()` | Display next steps |

### Error Handling

The script uses `set -e` to exit on any command failure. Each installation function:

1. Checks if component is already installed
2. Validates versions if applicable
3. Performs installation
4. Verifies successful installation
5. Logs all output
6. Exits with error if installation fails

### Color Coding

- 🟢 **GREEN** - Success messages
- 🔴 **RED** - Error messages
- 🟡 **YELLOW** - Warning messages
- 🔵 **BLUE** - Info messages
- 🟣 **MAGENTA** - Section headers
- 🔷 **CYAN** - Sub-section headers

## Security Considerations

### Sudo Usage

The script requests sudo privileges only when necessary:
- System package installation
- Docker service management
- User group modifications

### Downloaded Scripts

The script downloads and executes the golangci-lint installer:
```bash
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh
```

This is the official installation method. If concerned, review the script first:
```bash
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | less
```

### Package Sources

All packages come from:
- Ubuntu/Debian official repositories (`apt`)
- Go official packages (`go install`)
- Snap Store (Go language)

## Performance

Typical installation times (on Ubuntu 22.04 with good internet):

| Component | Time |
|-----------|------|
| System update | 30-60s |
| Go (snap) | 10-20s |
| Docker | 30-45s |
| Go tools | 60-120s |
| protoc | 10-15s |
| Go modules | 30-90s |
| **Total** | **3-5 minutes** |

Times vary based on:
- Internet connection speed
- System performance
- Package cache state
- Number of components already installed

## Integration with Make

The Makefile includes targets that assume prerequisites are installed:

```bash
# Verify prerequisites before building
make deps

# This calls:
go mod download
go mod verify
```

If prerequisites are missing, these will fail with helpful error messages.

## Uninstallation

To remove installed components:

```bash
# Remove Go
sudo snap remove go

# Remove Docker
sudo apt remove docker.io docker-compose
sudo apt autoremove

# Remove Go tools (manual)
rm -rf ~/go/bin/golangci-lint
rm -rf ~/go/bin/goimports
rm -rf ~/go/bin/protoc-gen-go
rm -rf ~/go/bin/protoc-gen-go-grpc

# Remove protoc
sudo apt remove protobuf-compiler
```

## Contributing

If you find issues with the installation script:

1. Check `scripts/install.log` for errors
2. Run with `--verbose` flag for detailed output
3. Open an issue with:
   - Your OS and version (`cat /etc/os-release`)
   - Script output
   - install.log contents

## License

This script is part of the MongoTron project and follows the same license.

## Support

- 📖 Documentation: See README.md
- 🐛 Issues: GitHub Issues
- 💬 Discussions: GitHub Discussions
- 📧 Contact: See README.md for contact information

---

**Last Updated:** October 3, 2025  
**Script Version:** 1.0.0  
**Tested On:** Ubuntu 22.04 LTS, Debian 12
