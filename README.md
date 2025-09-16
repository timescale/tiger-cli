# Tiger CLI

Tiger CLI is a command-line interface for managing TigerData Cloud Platform resources. Built as a single Go binary, it provides comprehensive tools for managing database services, VPCs, replicas, and related infrastructure components.

## Quick Start During Development
```bash
# Clone the repository
git clone https://github.com/tigerdata/tiger-cli.git
cd tiger-cli
git checkout <branch>

# Install the CLI
go install ./cmd/tiger

# (Optional) Set up the savannah-public endpoint
# for prod (default)
tiger config set api_url https://console.cloud.timescale.com/public/api/v1
# for dev
tiger config set api_url https://console.dev.timescale.com/public/api/v1
tiger config set console_url https://console.dev.timescale.com
tiger config set gateway_url https://console.dev.timescale.com/api
# for local
tiger config set api_url http://localhost:8080/public/api/v1

# Login to your account
tiger auth login

# explore the CLI
tiger --help
```

## Installation

### Install script
```bash
curl -fsSL https://tiger-cli-releases.s3.amazonaws.com/install/install.sh | sh
```

### Homebrew (macOS/Linux)
```bash
brew install timescale/tap/tiger-cli
```

### Go install
```bash
go install github.com/timescale/tiger-cli/cmd/tiger@latest
```

### Direct binary download
```bash
curl -LO https://github.com/timescale/tiger-cli/releases/latest/download/tiger-cli_$(uname -s)_$(uname -m).tar.gz
tar -xzf tiger-cli_$(uname -s)_$(uname -m).tar.gz
chmod +x tiger
mkdir -p ~/.local/bin && mv tiger ~/.local/bin/
```

### Debian/Ubuntu package
```bash
curl -LO https://github.com/timescale/tiger-cli/releases/latest/download/tiger-cli_linux_$(dpkg --print-architecture).deb
sudo dpkg -i tiger-cli_linux_$(dpkg --print-architecture).deb
```

## Usage

### Global Configuration

The CLI uses configuration stored in `~/.config/tiger/config.yaml` and supports environment variables with the `TIGER_` prefix.

```bash
# Show current configuration
tiger config show

# Set configuration values
tiger config set project_id proj-12345
tiger config set output json

# View help
tiger --help
```

### Environment Variables

- `TIGER_API_KEY`: TigerData API key for authentication
- `TIGER_API_URL`: Base URL for TigerData API (default: https://api.tigerdata.com/public/v1)
- `TIGER_PROJECT_ID`: Default project ID to use
- `TIGER_SERVICE_ID`: Default service ID to use
- `TIGER_CONFIG_DIR`: Configuration directory (default: ~/.config/tiger)
- `TIGER_OUTPUT`: Default output format (json, yaml, table)
- `TIGER_ANALYTICS`: Enable/disable usage analytics (true/false)

### Global Options

- `-o, --output`: Set output format (json, yaml, table)
- `--config-dir`: Path to configuration directory
- `--api-key`: Specify TigerData API key
- `--project-id`: Specify project ID
- `--service-id`: Specify service ID
- `--analytics`: Toggle analytics collection
- `-v, --version`: Show CLI version
- `-h, --help`: Show help information
- `--debug`: Enable debug logging

## Development

### Building from Source

```bash
git clone https://github.com/tigerdata/tiger-cli.git
cd tiger-cli
go build -o bin/tiger ./cmd/tiger
```

### Running Tests

```bash
# Run unit tests
go test ./...

# Run tests with verbose output
go test -v ./...
```

### Integration Testing

Integration tests execute real API calls against a TigerData environment to validate end-to-end functionality. These tests require valid credentials and will create/delete actual resources.

#### Setup

1. Copy the sample environment file:
   ```bash
   cp .env.sample .env
   ```

2. Edit `.env` with your actual credentials:
   ```bash
   TIGER_PUBLIC_KEY_INTEGRATION=your-public-key-here
   TIGER_SECRET_KEY_INTEGRATION=your-secret-key-here
   TIGER_PROJECT_ID_INTEGRATION=your-project-id-here
   TIGER_API_URL=http://localhost:8080/public/api/v1  # or production URL
   ```

#### Running Integration Tests

```bash
# Load environment variables and run all integration tests
export $(cat .env | xargs) && go test ./internal/tiger/cmd -v -run Integration

# Run specific integration test
export $(cat .env | xargs) && go test ./internal/tiger/cmd -v -run TestServiceLifecycleIntegration

# Integration tests will skip automatically if credentials are not set
go test ./internal/tiger/cmd -v -run Integration
```

#### What Integration Tests Cover

- **Authentication lifecycle**: Login with credentials, verify authentication, logout
- **Service management**: Create, list, describe, and delete database services  
- **Password management**: Update service passwords with keychain storage
- **Database connectivity**: Generate connection strings and execute psql commands
- **Output formats**: Validate JSON, YAML, and table output formats
- **Error handling**: Test authentication failures and resource cleanup

**Note**: Integration tests create and delete real services, which may incur costs. Use a development environment when possible.

## Project Structure

```
tiger-cli/
├── cmd/tiger/          # Main CLI entry point
├── internal/tiger/     # Internal packages
│   ├── config/         # Configuration management
│   ├── logging/        # Structured logging
│   └── cmd/           # CLI commands
├── specs/             # CLI specifications and documentation
└── bin/               # Built binaries
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.
