#!/bin/bash

################################################################################
# MongoTron Prerequisites Installation Script
# 
# This script automates the installation of all required tools and dependencies
# for developing and running the MongoTron blockchain monitoring microservice.
#
# Supported OS: Ubuntu 20.04+, Debian 11+
# Author: MongoTron Team
# Date: October 3, 2025
################################################################################

set -e  # Exit on error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
MAGENTA='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Configuration
MIN_GO_VERSION="1.21"
GO_VERSION="1.24.6"
GOLANGCI_LINT_VERSION="v1.55.2"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOG_FILE="${SCRIPT_DIR}/install.log"

# Flags
SKIP_DOCKER=false
SKIP_GO=false
SKIP_TOOLS=false
SKIP_DEPS=false
VERBOSE=false

################################################################################
# Helper Functions
################################################################################

log() {
    echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')]${NC} $*" | tee -a "${LOG_FILE}"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*" | tee -a "${LOG_FILE}"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $*" | tee -a "${LOG_FILE}"
}

log_info() {
    echo -e "${BLUE}[INFO]${NC} $*" | tee -a "${LOG_FILE}"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $*" | tee -a "${LOG_FILE}"
}

print_header() {
    echo ""
    echo -e "${MAGENTA}════════════════════════════════════════════════════════════════${NC}"
    echo -e "${MAGENTA}  $1${NC}"
    echo -e "${MAGENTA}════════════════════════════════════════════════════════════════${NC}"
    echo ""
}

print_section() {
    echo ""
    echo -e "${CYAN}────────────────────────────────────────────────────────────────${NC}"
    echo -e "${CYAN}  $1${NC}"
    echo -e "${CYAN}────────────────────────────────────────────────────────────────${NC}"
}

command_exists() {
    command -v "$1" >/dev/null 2>&1
}

version_ge() {
    # Compare versions: return 0 if $1 >= $2
    printf '%s\n%s' "$2" "$1" | sort -V -C
}

check_os() {
    if [[ -f /etc/os-release ]]; then
        . /etc/os-release
        OS=$NAME
        VER=$VERSION_ID
        log_info "Detected OS: $OS $VER"
        
        if [[ "$ID" != "ubuntu" && "$ID" != "debian" ]]; then
            log_warning "This script is designed for Ubuntu/Debian. Your OS may not be fully supported."
            read -p "Continue anyway? (y/N) " -n 1 -r
            echo
            if [[ ! $REPLY =~ ^[Yy]$ ]]; then
                exit 1
            fi
        fi
    else
        log_error "Cannot detect OS. /etc/os-release not found."
        exit 1
    fi
}

check_root() {
    if [[ $EUID -eq 0 ]]; then
        log_error "This script should NOT be run as root/sudo directly."
        log_error "It will request sudo privileges when needed."
        exit 1
    fi
}

check_sudo() {
    if ! sudo -n true 2>/dev/null; then
        log_info "This script requires sudo privileges for some operations."
        log_info "You may be prompted for your password."
        sudo -v
    fi
}

################################################################################
# Installation Functions
################################################################################

update_system() {
    print_section "Updating System Packages"
    
    log "Updating package lists..."
    sudo apt update -qq | tee -a "${LOG_FILE}"
    
    log "Installing essential build tools..."
    sudo apt install -y -qq \
        curl \
        wget \
        git \
        build-essential \
        ca-certificates \
        gnupg \
        lsb-release \
        software-properties-common \
        apt-transport-https \
        | tee -a "${LOG_FILE}"
    
    log_success "System packages updated"
}

install_go() {
    if [[ "$SKIP_GO" == "true" ]]; then
        log_info "Skipping Go installation (--skip-go)"
        return
    fi
    
    print_section "Installing Go"
    
    # Check if Go is already installed
    if command_exists go; then
        CURRENT_GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
        log_info "Go is already installed: $CURRENT_GO_VERSION"
        
        if version_ge "$CURRENT_GO_VERSION" "$MIN_GO_VERSION"; then
            log_success "Go version $CURRENT_GO_VERSION meets minimum requirement ($MIN_GO_VERSION)"
            return
        else
            log_warning "Go version $CURRENT_GO_VERSION is below minimum ($MIN_GO_VERSION)"
            log "Upgrading Go..."
        fi
    fi
    
    # Install Go using snap (simplest method for Ubuntu)
    log "Installing Go $GO_VERSION via snap..."
    sudo snap install go --classic | tee -a "${LOG_FILE}"
    
    # Verify installation
    if command_exists go; then
        INSTALLED_VERSION=$(go version | awk '{print $3}')
        log_success "Go installed successfully: $INSTALLED_VERSION"
    else
        log_error "Go installation failed"
        exit 1
    fi
    
    # Set up Go environment
    setup_go_env
}

setup_go_env() {
    log "Configuring Go environment..."
    
    GOPATH="${HOME}/go"
    GOBIN="${GOPATH}/bin"
    
    # Create Go directories
    mkdir -p "$GOPATH"/{src,bin,pkg}
    
    # Add to PATH if not already there
    SHELL_RC="${HOME}/.bashrc"
    if [[ -f "${HOME}/.zshrc" ]]; then
        SHELL_RC="${HOME}/.zshrc"
    fi
    
    if ! grep -q "GOPATH" "$SHELL_RC"; then
        log "Adding Go paths to $SHELL_RC"
        cat >> "$SHELL_RC" << 'EOF'

# Go environment
export GOPATH=$HOME/go
export PATH=$PATH:$GOPATH/bin
EOF
        export GOPATH="$HOME/go"
        export PATH="$PATH:$GOPATH/bin"
    fi
    
    log_success "Go environment configured"
}

install_go_tools() {
    if [[ "$SKIP_TOOLS" == "true" ]]; then
        log_info "Skipping Go tools installation (--skip-tools)"
        return
    fi
    
    print_section "Installing Go Development Tools"
    
    # Ensure GOPATH is set
    if [[ -z "$GOPATH" ]]; then
        export GOPATH="${HOME}/go"
        export PATH="$PATH:$GOPATH/bin"
    fi
    
    # Install goimports
    log "Installing goimports..."
    go install golang.org/x/tools/cmd/goimports@latest 2>&1 | tee -a "${LOG_FILE}"
    
    # Install golangci-lint
    log "Installing golangci-lint $GOLANGCI_LINT_VERSION..."
    curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | \
        sh -s -- -b "$GOPATH/bin" "$GOLANGCI_LINT_VERSION" 2>&1 | tee -a "${LOG_FILE}"
    
    # Install protoc-gen-go
    log "Installing protoc-gen-go..."
    go install google.golang.org/protobuf/cmd/protoc-gen-go@latest 2>&1 | tee -a "${LOG_FILE}"
    
    # Install protoc-gen-go-grpc
    log "Installing protoc-gen-go-grpc..."
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest 2>&1 | tee -a "${LOG_FILE}"
    
    # Install govulncheck
    log "Installing govulncheck..."
    go install golang.org/x/vuln/cmd/govulncheck@latest 2>&1 | tee -a "${LOG_FILE}"
    
    # Install other useful tools
    log "Installing additional Go tools..."
    go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest 2>&1 | tee -a "${LOG_FILE}" || true
    
    log_success "Go development tools installed"
}

install_protoc() {
    print_section "Installing Protocol Buffers Compiler"
    
    if command_exists protoc; then
        PROTOC_VERSION=$(protoc --version | awk '{print $2}')
        log_info "protoc is already installed: $PROTOC_VERSION"
        return
    fi
    
    log "Installing protobuf-compiler..."
    sudo apt install -y protobuf-compiler 2>&1 | tee -a "${LOG_FILE}"
    
    if command_exists protoc; then
        PROTOC_VERSION=$(protoc --version | awk '{print $2}')
        log_success "protoc installed successfully: $PROTOC_VERSION"
    else
        log_error "protoc installation failed"
        exit 1
    fi
}

install_docker() {
    if [[ "$SKIP_DOCKER" == "true" ]]; then
        log_info "Skipping Docker installation (--skip-docker)"
        return
    fi
    
    print_section "Installing Docker"
    
    # Check if Docker is already installed
    if command_exists docker; then
        DOCKER_VERSION=$(docker --version | awk '{print $3}' | sed 's/,//')
        log_info "Docker is already installed: $DOCKER_VERSION"
        
        # Check if user is in docker group
        if groups | grep -q docker; then
            log_success "User is already in docker group"
            return
        else
            log "Adding user to docker group..."
            sudo usermod -aG docker "$USER"
            log_success "User added to docker group (restart shell to apply)"
            return
        fi
    fi
    
    log "Installing Docker..."
    sudo apt install -y docker.io 2>&1 | tee -a "${LOG_FILE}"
    
    # Start and enable Docker service
    log "Starting Docker service..."
    sudo systemctl start docker 2>&1 | tee -a "${LOG_FILE}"
    sudo systemctl enable docker 2>&1 | tee -a "${LOG_FILE}"
    
    # Add user to docker group
    log "Adding user to docker group..."
    sudo usermod -aG docker "$USER"
    
    if command_exists docker; then
        DOCKER_VERSION=$(docker --version | awk '{print $3}')
        log_success "Docker installed successfully: $DOCKER_VERSION"
        log_warning "You need to logout/login or run 'newgrp docker' to use Docker without sudo"
    else
        log_error "Docker installation failed"
        exit 1
    fi
}

install_docker_compose() {
    if [[ "$SKIP_DOCKER" == "true" ]]; then
        return
    fi
    
    print_section "Installing Docker Compose"
    
    if command_exists docker-compose; then
        COMPOSE_VERSION=$(docker-compose --version | awk '{print $3}' | sed 's/,//')
        log_info "Docker Compose is already installed: $COMPOSE_VERSION"
        return
    fi
    
    log "Installing Docker Compose..."
    sudo apt install -y docker-compose 2>&1 | tee -a "${LOG_FILE}"
    
    if command_exists docker-compose; then
        COMPOSE_VERSION=$(docker-compose --version | awk '{print $3}')
        log_success "Docker Compose installed successfully: $COMPOSE_VERSION"
    else
        log_error "Docker Compose installation failed"
        exit 1
    fi
}

install_project_deps() {
    if [[ "$SKIP_DEPS" == "true" ]]; then
        log_info "Skipping project dependencies (--skip-deps)"
        return
    fi
    
    print_section "Installing Project Dependencies"
    
    # Check if we're in a Go project directory
    if [[ ! -f "go.mod" ]]; then
        log_warning "go.mod not found. Skipping dependency installation."
        log_info "Run 'go mod download' manually in the project directory."
        return
    fi
    
    log "Downloading Go module dependencies..."
    go mod download 2>&1 | tee -a "${LOG_FILE}"
    
    log "Tidying Go modules..."
    go mod tidy 2>&1 | tee -a "${LOG_FILE}"
    
    log_success "Project dependencies installed"
}

install_optional_tools() {
    print_section "Installing Optional Development Tools"
    
    log "Installing additional utilities (jq, tree, htop, etc.)..."
    sudo apt install -y -qq \
        jq \
        tree \
        htop \
        net-tools \
        vim \
        2>&1 | tee -a "${LOG_FILE}" || true
    
    log_success "Optional tools installed"
}

################################################################################
# Verification Functions
################################################################################

verify_installation() {
    print_section "Verifying Installation"
    
    local all_good=true
    
    # Check Go
    if command_exists go; then
        GO_VER=$(go version | awk '{print $3}')
        log_success "✓ Go: $GO_VER"
    else
        log_error "✗ Go: Not found"
        all_good=false
    fi
    
    # Check Docker
    if [[ "$SKIP_DOCKER" != "true" ]]; then
        if command_exists docker; then
            DOCKER_VER=$(docker --version | awk '{print $3}' | sed 's/,//')
            log_success "✓ Docker: $DOCKER_VER"
        else
            log_error "✗ Docker: Not found"
            all_good=false
        fi
        
        if command_exists docker-compose; then
            COMPOSE_VER=$(docker-compose --version | awk '{print $3}' | sed 's/,//')
            log_success "✓ Docker Compose: $COMPOSE_VER"
        else
            log_error "✗ Docker Compose: Not found"
            all_good=false
        fi
    fi
    
    # Check protoc
    if command_exists protoc; then
        PROTOC_VER=$(protoc --version | awk '{print $2}')
        log_success "✓ Protocol Buffers: $PROTOC_VER"
    else
        log_error "✗ Protocol Buffers: Not found"
        all_good=false
    fi
    
    # Check Go tools
    export PATH="$PATH:$HOME/go/bin"
    
    if command_exists golangci-lint; then
        LINT_VER=$(golangci-lint --version 2>/dev/null | head -1 | awk '{print $4}' || echo "installed")
        log_success "✓ golangci-lint: $LINT_VER"
    else
        log_warning "✗ golangci-lint: Not found in PATH"
    fi
    
    if command_exists goimports; then
        log_success "✓ goimports: installed"
    else
        log_warning "✗ goimports: Not found in PATH"
    fi
    
    if command_exists protoc-gen-go; then
        log_success "✓ protoc-gen-go: installed"
    else
        log_warning "✗ protoc-gen-go: Not found in PATH"
    fi
    
    if command_exists protoc-gen-go-grpc; then
        log_success "✓ protoc-gen-go-grpc: installed"
    else
        log_warning "✗ protoc-gen-go-grpc: Not found in PATH"
    fi
    
    if [[ "$all_good" == "true" ]]; then
        log_success "All core components verified successfully!"
    else
        log_error "Some components failed verification. Check the log above."
        return 1
    fi
}

################################################################################
# Post-Installation
################################################################################

print_post_install_info() {
    print_header "Installation Complete!"
    
    cat << EOF

${GREEN}✅ MongoTron Prerequisites Successfully Installed!${NC}

${CYAN}📦 Installed Components:${NC}
EOF
    
    if command_exists go; then
        echo "  • Go: $(go version | awk '{print $3}')"
    fi
    
    if command_exists docker && [[ "$SKIP_DOCKER" != "true" ]]; then
        echo "  • Docker: $(docker --version | awk '{print $3}' | sed 's/,//')"
        echo "  • Docker Compose: $(docker-compose --version | awk '{print $3}' | sed 's/,//')"
    fi
    
    if command_exists protoc; then
        echo "  • Protocol Buffers: $(protoc --version | awk '{print $2}')"
    fi
    
    echo "  • golangci-lint, goimports, protoc-gen-go, protoc-gen-go-grpc"
    
    cat << EOF

${YELLOW}⚠️  Important Next Steps:${NC}

1. ${CYAN}Reload your shell configuration:${NC}
   ${GREEN}source ~/.bashrc${NC}
   ${YELLOW}# OR start a new terminal session${NC}

2. ${CYAN}Apply Docker group membership (if Docker was installed):${NC}
   ${GREEN}newgrp docker${NC}
   ${YELLOW}# OR logout and login again${NC}

3. ${CYAN}Verify Go tools are in PATH:${NC}
   ${GREEN}export PATH=\$PATH:\$HOME/go/bin${NC}
   ${GREEN}which golangci-lint${NC}

4. ${CYAN}Start building MongoTron:${NC}
   ${GREEN}cd $(pwd)${NC}
   ${GREEN}make deps${NC}
   ${GREEN}make build${NC}

${CYAN}📚 Documentation:${NC}
  • Installation log: ${LOG_FILE}
  • README: README.md
  • Prerequisites: PREREQUISITES_INSTALLED.md

${CYAN}🚀 Quick Start Commands:${NC}
  ${GREEN}make build${NC}       - Build all binaries
  ${GREEN}make test${NC}        - Run tests
  ${GREEN}make run${NC}         - Run locally
  ${GREEN}make docker-run${NC}  - Run with Docker

${GREEN}Happy Coding! 🎉${NC}

EOF
}

################################################################################
# Usage and Main
################################################################################

usage() {
    cat << EOF
Usage: $0 [OPTIONS]

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
    $0                      # Install everything
    $0 --skip-docker        # Install without Docker
    $0 --skip-deps          # Install without downloading dependencies
    $0 -y                   # Install everything without confirmations

EOF
}

main() {
    # Parse command line arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                usage
                exit 0
                ;;
            --skip-docker)
                SKIP_DOCKER=true
                shift
                ;;
            --skip-go)
                SKIP_GO=true
                shift
                ;;
            --skip-tools)
                SKIP_TOOLS=true
                shift
                ;;
            --skip-deps)
                SKIP_DEPS=true
                shift
                ;;
            --verbose)
                VERBOSE=true
                set -x
                shift
                ;;
            -y|--yes)
                # Auto-confirm (future use)
                shift
                ;;
            *)
                log_error "Unknown option: $1"
                usage
                exit 1
                ;;
        esac
    done
    
    # Initialize log file
    echo "MongoTron Prerequisites Installation" > "$LOG_FILE"
    echo "Started: $(date)" >> "$LOG_FILE"
    echo "========================================" >> "$LOG_FILE"
    
    print_header "MongoTron Prerequisites Installer"
    
    log_info "Installation log: $LOG_FILE"
    
    # Preflight checks
    check_root
    check_os
    check_sudo
    
    # Installation steps
    update_system
    install_go
    install_go_tools
    install_protoc
    install_docker
    install_docker_compose
    install_project_deps
    install_optional_tools
    
    # Verification
    verify_installation
    
    # Post-installation info
    print_post_install_info
    
    log_info "Installation completed: $(date)"
}

# Run main function
main "$@"
