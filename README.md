# HTTP Mirror

A Kubernetes-native solution for mirroring HTTP directory listings and serving them via a web server.

## Architecture

HTTP Mirror uses a two-component approach following Kubernetes best practices:

- **Web Server** (Deployment): Always-running Go web server that serves mirrored files with custom directory listings
- **Updater** (CronJob): Scheduled Go binary that downloads/updates files from configured HTTP directory listings

Both components share data via a Kubernetes PersistentVolume and are configured through a shared ConfigMap.

## Features

✅ **Smart mirroring** - HTTP HEAD requests to check file changes before download  
✅ **Built-in file server** - Custom directory listings with search and filtering  
✅ **Kubernetes-native** - CronJob scheduling with timezone support  
✅ **Rate limiting** - Configurable download speeds to be respectful  
✅ **Multi-target support** - Mirror multiple sites with individual configurations  
✅ **Change detection** - Only download files that have actually changed  
✅ **Resumable downloads** - Continue interrupted downloads  
✅ **Health checks** - Kubernetes-ready liveness and readiness probes  
✅ **Monitoring** - Prometheus metrics and ServiceMonitor support

## Quick Start

### Using Helm (Recommended)

```bash
# Add the Helm repository
helm repo add jhofer-cloud https://charts.jhofer.org
helm repo update

# Install with basic configuration
helm install my-mirror jhofer-cloud/http-mirror \
  --set targets[0].name=example \
  --set targets[0].url=http://example.com/files/ \
  --set ingress.enabled=true \
  --set ingress.hosts[0].host=mirror.example.com
```

### Using Docker

```bash
# Create a config file
cat > config.json << EOF
{
  "targets": [
    {
      "name": "example",
      "url": "http://example.com/files/"
    }
  ]
}
EOF

# Run updater once
docker run --rm -v $(pwd)/data:/data -v $(pwd)/config.json:/config/config.json \
  ghcr.io/jhofer-cloud/http-mirror-updater:latest

# Start web server
docker run -d -p 8080:8080 -v $(pwd)/data:/data -v $(pwd)/config.json:/config/config.json \
  ghcr.io/jhofer-cloud/http-mirror-server:latest
```

## Configuration

See: <https://github.com/JHOFER-Cloud/helm-charts/tree/main/charts/http-mirror>

## Development

### Prerequisites

- Go 1.21+
- Docker
- Kubernetes cluster (for testing)

### Build Locally

```bash
# Clone the repository
git clone https://github.com/jhofer-cloud/http-mirror.git
cd http-mirror

# Download dependencies
go mod download

# Build binaries
go build -o bin/server ./cmd/server
go build -o bin/updater ./cmd/updater

# Run tests
go test -v ./...
```

### Build Docker Images

```bash
# Build server image
docker build -f Dockerfile.server -t http-mirror-server .

# Build updater image
docker build -f Dockerfile.updater -t http-mirror-updater .
```

## Directory Structure

```
http-mirror/
├── cmd/
│   ├── server/           # Web server binary
│   └── updater/          # Mirror updater binary
├── pkg/
│   ├── config/           # Shared configuration
│   ├── http/             # HTTP client with rate limiting
│   ├── files/            # File handler with directory listings
│   └── mirror/           # Core mirroring logic
├── Dockerfile.server     # Server container image
├── Dockerfile.updater    # Updater container image
└── .github/workflows/    # CI/CD pipelines
```

## How It Works

1. **Configuration**: Targets are configured via ConfigMap or environment variables
2. **Scheduling**: CronJob triggers the updater at configured times
3. **Smart Mirroring**: Updater checks remote file timestamps/ETags before downloading
4. **File Storage**: Files are stored in a shared PersistentVolume
5. **Web Serving**: Always-running server serves files with auto-generated directory listings
6. **Monitoring**: Health checks and optional Prometheus metrics

## Use Cases

- **Archive Preservation**: Mirror important file repositories before they disappear
- **Local Caching**: Speed up access to frequently used files
- **Offline Access**: Make remote files available when internet is limited
- **Compliance**: Keep local copies of compliance-related downloads
- **Development**: Mirror dependency archives for air-gapped environments

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests if applicable
5. Run `go test ./...` and `go vet ./...`
6. Submit a pull request

## License

[Apache License 2.0](LICENSE)

## Support

- 📝 [Documentation](https://github.com/jhofer-cloud/http-mirror/wiki)
- 🐛 [Issues](https://github.com/jhofer-cloud/http-mirror/issues)
- 💬 [Discussions](https://github.com/jhofer-cloud/http-mirror/discussions)
