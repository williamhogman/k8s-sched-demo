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
            skaffold
            kubernetes-helm            

            google-cloud-sdk
            
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
          '';
        };
      }
    );
} 