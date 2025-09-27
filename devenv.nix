{pkgs, ...}: {
  # Enable Go development environment
  languages.go = {
    enable = true;
  };

  # Additional development tools
  packages = with pkgs; [
    # Go tools
    golangci-lint
    gotools
    go-outline
    gopls
  ];

  # Environment variables
  env = {
    GO111MODULE = "on";
    GOPROXY = "https://proxy.golang.org,direct";
  };

  # Git hooks
  git-hooks.hooks = {
    gofmt.enable = true;
    govet.enable = true;
  };

  # Scripts for HTTP Mirror development workflow
  scripts = {
    # Build all three components
    d-build-all.exec = ''
      mkdir -p bin
      go build -o bin/testserver ./cmd/testserver
      go build -o bin/updater ./cmd/updater
      go build -o bin/server ./cmd/server
      echo "✅ Built: testserver, updater, server"
    '';

    # Development workflow commands
    d-testserver.exec = "echo 'Starting mock HTTP server on :8080...' && ./bin/testserver";
    d-mirror.exec = "echo 'Running updater to mirror files...' && ./bin/updater -config test-config.json";
    d-server.exec = "echo 'Starting web server on :3000...' && ./bin/server -config test-config.json";

    # Full automated demo
    d-demo.exec = "./test-local.sh demo";

    # Testing
    d-test.exec = "go test ./...";
    d-test-integration.exec = "go test -v ./integration_test.go";

    # Development utilities
    d-lint.exec = "golangci-lint run";
    d-tidy.exec = "go mod tidy";
    d-dev-check.exec = "go fmt ./... && golangci-lint run && go test ./...";

    # Check what got mirrored
    d-check-files.exec = "echo 'Mirrored files:' && find test-data -type f -exec ls -la {} \\;";

    # Health checks
    d-health.exec = "curl -s http://localhost:3000/health | jq || curl -s http://localhost:3000/health";
    d-metrics.exec = "curl -s http://localhost:3000/metrics";

    # Cleanup
    d-clean.exec = ''
      rm -rf bin/ test-data/
      pkill -f "testserver|updater|server.*test-config" 2>/dev/null || true
      echo "🧹 Cleaned up"
    '';
  };

  # Development processes (optional - uncomment if you want auto-restart)
  # processes = {
  #   testserver.exec = "./bin/testserver";
  #   server.exec = "./bin/server -config test-config.json";
  # };

  enterShell = ''
    mkdir -p bin test-data
    echo "🔧 HTTP Mirror - Archive HTTP directory listings"
    echo ""
    echo "Workflow: d-testserver → d-mirror → d-server"
    echo "  1. d-testserver  - Mock HTTP server (:8080)"
    echo "  2. d-mirror      - Download files once (like wget)"
    echo "  3. d-server      - Serve mirrored files (:3000)"
    echo ""
    echo "Quick start: d-build-all && d-demo"
  '';
}
