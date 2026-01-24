# ThisIsFine 🐳

A simple, self-contained Docker service status dashboard written in Go. Monitor which of your Docker Compose services are running at a glance.

![Dashboard Preview](https://img.shields.io/badge/status-active-success)

## Features

- 🚀 Simple web-based status dashboard
- 📊 Visual status indicators for each service
- 🔄 Real-time service status checking via `docker ps`
- ⚙️ Configurable port and service list
- 🎨 Clean, responsive UI that scales well with 20+ services
- 🔧 Runs as a systemd service

## Requirements

- Go 1.25 or later
- Docker installed and running
- Docker Compose services with standard labels

## Installation

### 1. Clone the repository

```bash
git clone https://github.com/its-the-vibe/ThisIsFine.git
cd ThisIsFine
```

### 2. Build the application

```bash
go build -o ThisIsFine
```

### 3. Create configuration file

Copy the example configuration and customize it for your services:

```bash
cp config.json.example config.json
```

Edit `config.json` to list your expected services:

```json
{
  "port": 8080,
  "services": [
    "its-the-vibe/rate-my",
    "its-the-vibe/another-service",
    "its-the-vibe/yet-another-service"
  ]
}
```

**Note:** The service paths should be in the format `orgname/servicename` (the last two path segments of the `com.docker.compose.project.working_dir` label from your Docker Compose containers).

## Usage

### Running Manually

```bash
./ThisIsFine
```

Or with a custom configuration file:

```bash
./ThisIsFine -config /path/to/config.json
```

Then open your browser to `http://localhost:8080` (or your configured port).

### Running as a systemd Service

1. Copy the binary and configuration to the installation directory:

```bash
sudo mkdir -p /opt/thisisfine
sudo cp ThisIsFine /opt/thisisfine/
sudo cp config.json /opt/thisisfine/
```

2. Update the systemd service file if needed (edit `thisisfine.service` to match your user and paths).

3. Install the systemd service:

```bash
sudo cp thisisfine.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable thisisfine
sudo systemctl start thisisfine
```

4. Check the status:

```bash
sudo systemctl status thisisfine
```

## How It Works

1. The application reads the list of expected services from `config.json`
2. When you access the dashboard, it executes `docker ps --format json`
3. For each running container, it extracts the `com.docker.compose.project.working_dir` label
4. It compares the expected services against the running services
5. The dashboard displays each service with a visual indicator:
   - 🟢 Green = Service is running
   - 🔴 Red = Service is stopped

## Configuration

The `config.json` file supports the following options:

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `port` | integer | 8080 | The port on which the web server listens |
| `services` | array of strings | [] | List of expected service working directories |

## Development

### Build

```bash
go build
```

### Run

```bash
go run main.go
```

## License

MIT License - feel free to use this for your own Docker service monitoring needs.

## Contributing

Pull requests are welcome! For major changes, please open an issue first to discuss what you would like to change.
