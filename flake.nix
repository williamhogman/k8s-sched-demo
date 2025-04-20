{
  description = "K8s Scheduler Demo - A Golang project demonstrating custom Kubernetes scheduling";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    # For direnv integration
    nix-direnv.url = "github:nix-community/nix-direnv";
  };

  outputs = { self, nixpkgs, flake-utils, nix-direnv, ... }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs {
          inherit system;
          overlays = [ ];
        };

        # Go version from go.mod (using the latest stable Go version)
        go = pkgs.go_1_23;

      in rec {
        # Development environment
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            # Go and related tools
            go
            gopls
            gotools
            go-tools
            golangci-lint
            delve # Go debugger
            
            # Protocol Buffers
            buf
            protobuf
            protoc-gen-go
            protoc-gen-connect-go

            # HTTP testing tools
            xh # HTTPie for X
            
            # Kubernetes tools
            kubectl
            kubernetes-helm
            
            # Database
            redis   # Add Redis for state management
            
            # Misc tools
            gnumake
            ncurses # For the tput command
            netcat  # For the nc command
            jq      # For JSON processing
            
            # Direnv support
            direnv
            nix-direnv.packages.${system}.default
          ];

          # Set environment variables
          shellHook = ''
            # Create local bin directory if it doesn't exist
            mkdir -p ./.bin
            
            # Add local bin to PATH
            export PATH=$PWD/.bin:$PATH
            
            # Go setup
            export GOPATH=$PWD/.go
            export GOBIN=$PWD/.bin
            export GOCACHE=$PWD/.cache/go-build
            export GOENV=$PWD/.config/go/env
            
            # Buf setup
            export BUF_CACHE_DIR=$PWD/.cache/buf
            
            # Colorful PS1
            export PS1="\[\033[01;32m\]k8s-sched-demo\[\033[00m\]:\[\033[01;34m\]\w\[\033[00m\]\$ "
            
            echo ""
            echo "🚀 Welcome to K8s Scheduler Demo development environment!"
            echo "📦 Go $(go version | awk '{print $3}') is available"
            echo "🧰 Development tools installed: buf, protoc, gopls, golangci-lint, xh"
            echo "💾 Redis server available: $(redis-server --version | head -n 1)"
            echo ""
            echo "Quick commands:"
            echo "  make build         - Build the project"
            echo "  ./demo.sh          - Run the demo"
            echo "  buf generate       - Generate protocol buffer files"
            echo ""
          '';
        };

        # Package definition
        packages.default = pkgs.buildGoModule rec {
          pname = "k8s-sched-demo";
          version = "0.1.0";
          
          src = ./.;
          
          # Vendored dependencies hash
          # Update this with: go mod vendor && nix-hash --type sha256 --base32 vendor
          vendorHash = null; # Set to null for the first build to get the correct hash
          
          # Generate Protobuf files during build
          preBuild = ''
            mkdir -p gen
            ${pkgs.buf}/bin/buf generate
          '';
          
          # Don't include vendor directory
          excludedPackages = [ ];
          
          meta = with pkgs.lib; {
            description = "Kubernetes Scheduler Demo";
            homepage = "https://github.com/williamhogman/k8s-sched-demo";
            license = licenses.mit; # Adjust according to your project's license
            maintainers = with maintainers; [ ];
          };
        };

        # Apps definition for running with 'nix run'
        apps.default = flake-utils.lib.mkApp {
          drv = packages.default;
          name = "k8s-sched-demo";
        };
      }
    );
} 