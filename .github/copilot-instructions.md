# ThisIsFine - Copilot Instructions

## Project Overview
ThisIsFine is a simple, self-contained Docker service status dashboard written in Go. It monitors Docker Compose services and displays their status in a web-based dashboard.

## Tech Stack
- **Language**: Go 1.25.5+
- **Runtime**: Docker (required for monitoring containers)
- **Testing**: Go's built-in testing framework
- **Deployment**: Standalone binary or systemd service

## Project Structure
```
.
├── main.go                      # Main application code
├── main_test.go                 # Unit tests
├── go.mod                       # Go module definition
├── config.json.example          # Example configuration file
├── thisisfine.service.example   # Example systemd service file
└── README.md                    # Documentation
```

## Build and Test Commands
```bash
# Build the application
go build -o ThisIsFine

# Run the application
go run main.go
# Or with custom config:
go run main.go -config /path/to/config.json

# Run tests
go test -v

# Run specific test
go test -v -run TestExtractWorkingDir
```

## Code Style and Conventions
- Follow standard Go conventions and `gofmt` formatting
- Use descriptive variable names
- Add godoc comments for exported functions and types
- Keep functions focused and single-purpose
- Use table-driven tests for comprehensive test coverage
- Error messages should be lowercase and not end with punctuation (Go convention)
- Chain errors using `fmt.Errorf` with `%w` for error wrapping

### Example Code Style
```go
// Good: Table-driven test
func TestExtractWorkingDir(t *testing.T) {
    tests := []struct {
        name     string
        labels   string
        expected string
    }{
        {
            name:     "Full path with working_dir label",
            labels:   "com.docker.compose.project.working_dir=/home/ubuntu/github/its-the-vibe/rate-my",
            expected: "its-the-vibe/rate-my",
        },
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := extractWorkingDir(tt.labels)
            if result != tt.expected {
                t.Errorf("extractWorkingDir(%q) = %q, want %q", tt.labels, result, tt.expected)
            }
        })
    }
}

// Good: Error handling with wrapping
if err := loadConfig(*configPath); err != nil {
    return fmt.Errorf("loading config: %w", err)
}
```

## Configuration
- The application reads configuration from `config.json` (or custom path via `-config` flag)
- Configuration includes:
  - `port`: Web server port (default: 8080)
  - `services`: Array of service paths to monitor (format: "orgname/servicename")
- Service paths should match the last two segments of the Docker container's `com.docker.compose.project.working_dir` label

## Testing Requirements
- Write table-driven tests for new parsing/extraction functions
- Test both happy paths and edge cases (empty strings, missing data, malformed input)
- Ensure tests are descriptive with clear test names
- Run `go test -v` to verify all tests pass before submitting changes

## Contribution Guidelines
- Keep changes minimal and focused
- Maintain backward compatibility
- Update README.md if adding new features or changing configuration
- Ensure all tests pass before committing
- Format code with `gofmt` before committing

## Security and Safety
- Never commit sensitive data or credentials to the repository
- Be cautious with user input and validate configuration data
- The application executes Docker commands - ensure proper error handling
- When modifying Docker command execution, validate command arguments

## What NOT to Change
- Do not modify the service extraction logic without comprehensive tests
- Do not change the Docker label format (`com.docker.compose.project.working_dir`)
- Do not remove existing test cases
- Avoid changing the web dashboard HTML template unless specifically requested

## Dependencies
- Minimal external dependencies (only standard library)
- Requires Docker to be installed and running on the host system
- No need to add dependencies unless absolutely necessary for the task
