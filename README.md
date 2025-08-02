# Tiger CLI

Tiger CLI is a command-line interface for managing TigerData Cloud Platform resources. Built as a single Go binary, it provides comprehensive tools for managing database services, VPCs, replicas, and related infrastructure components.

## Installation

### Homebrew (macOS/Linux)
```bash
brew install tigerdata/tap/tiger
```

### Go install
```bash
go install github.com/tigerdata/tiger-cli/cmd/tiger@latest
```

### Direct binary download
```bash
curl -L https://github.com/tigerdata/tiger-cli/releases/latest/download/tiger-$(uname -s)-$(uname -m) -o tiger
chmod +x tiger
mv tiger /usr/local/bin/
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
go test ./...
```

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

[License information to be added]