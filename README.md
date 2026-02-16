# ThisIsFine 🐳

A simple, self-contained service status dashboard written in Go. Monitor both Docker Compose services and systemd services at a glance.

![Dashboard Preview](https://img.shields.io/badge/status-active-success)

## Features

- 🚀 Simple web-based status dashboard
- 🐳 Docker Compose service monitoring
- ⚙️ systemd service monitoring  
- 📊 Visual status indicators for each service
- 🔄 Real-time service status checking
- 💻 Resource usage stats for Docker containers (CPU, Memory)
- 🎨 Clean, responsive UI that scales well with 20+ services
- 🔧 Configurable port and service lists
- 📡 REST API endpoints for integration

## Requirements

- Go 1.25 or later
- Docker installed and running (for Docker service monitoring)
- systemd (for systemd service monitoring)

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
  "dockerServices": [
    "its-the-vibe/rate-my",
    "its-the-vibe/another-service",
    "its-the-vibe/yet-another-service"
  ],
  "systemdServices": [
    "nginx",
    "postgresql",
    "redis"
  ]
}
```

**Note:**
- Docker service paths should be in the format `orgname/servicename` (the last two path segments of the `com.docker.compose.project.working_dir` label from your Docker Compose containers).
- systemd services should be the service unit names (e.g., `nginx`, `postgresql.service`).

**Backward Compatibility:** The old `services` field is still supported for backward compatibility and will be automatically migrated to `dockerServices` when the config is loaded.

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

### API Endpoints

The application provides the following HTTP endpoints:

- **`GET /`** - Web dashboard showing the status of configured services
- **`GET /ps`** - JSON API endpoint that returns the output of `docker ps --format json`
- **`GET /stats`** - JSON API endpoint that returns resource usage statistics for Docker services
- **`GET /systemctl-is-active?services=service1,service2`** - JSON API endpoint that returns systemd service statuses

#### `/ps` Endpoint

The `/ps` endpoint provides direct access to Docker container information in JSON format. This is useful for monitoring tools and integrations that need programmatic access to container status.

**Example request:**
```bash
curl http://localhost:8080/ps
```

**Example response:**
```json
{"Command":"\"nginx -g 'daemon of…\"","CreatedAt":"2026-02-03 14:23:45 -0800 PST","ID":"a1b2c3d4e5f6","Image":"nginx:latest","Labels":"com.docker.compose.project=myproject","LocalVolumes":"2","Mounts":"/var/lib/docker/volumes/...","Names":"web-server","Networks":"bridge","Ports":"80/tcp, 443/tcp","RunningFor":"2 hours ago","Size":"0B (virtual 187MB)","State":"running","Status":"Up 2 hours"}
```

**Response format:** Each line is a separate JSON object representing a container (newline-delimited JSON).

**Error handling:** Returns HTTP 500 with error message if Docker is unavailable or the command fails.

#### `/stats` Endpoint

The `/stats` endpoint provides resource usage statistics for Docker services configured in the config file.

**Example request:**
```bash
curl http://localhost:8080/stats
```

**Example response:**
```json
[
  {
    "name": "its-the-vibe/rate-my",
    "running": true,
    "cpuPerc": "0.50%",
    "memUsage": "128MiB / 2GiB"
  },
  {
    "name": "its-the-vibe/service-1",
    "running": false
  }
]
```

#### `/systemctl-is-active` Endpoint

The `/systemctl-is-active` endpoint checks the status of systemd services using `systemctl is-active`.

**Example request:**
```bash
curl "http://localhost:8080/systemctl-is-active?services=nginx,postgresql,redis"
```

**Example response:**
```json
{
  "nginx": "active",
  "postgresql": "active",
  "redis": "inactive"
}
```

**Query parameters:**
- `services` (required): Comma-separated list of systemd service names to check

**Response format:** JSON object mapping each service name to its status (`active`, `inactive`, `failed`, etc.).

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

### Docker Service Monitoring

1. The application reads the list of expected Docker services from `dockerServices` in `config.json`
2. When you access the dashboard, it executes `docker ps --format json`
3. For each running container, it extracts the `com.docker.compose.project.working_dir` label
4. It compares the expected services against the running services
5. For Docker services, resource stats (CPU, memory) can be displayed by clicking "Show Resource Stats"

### systemd Service Monitoring

1. The application reads the list of expected systemd services from `systemdServices` in `config.json`
2. It uses `systemctl is-active <service>` to check the status of each service
3. Services are displayed with their current status (active, inactive, failed, etc.)

The dashboard displays each service with visual indicators:
- 🟢 Green = Service is running/active
- 🔴 Red = Service is stopped/inactive
- Badge shows service type (docker or systemd)

## Configuration

The `config.json` file supports the following options:

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `port` | integer | 8080 | The port on which the web server listens |
| `dockerServices` | array of strings | [] | List of Docker Compose service paths to monitor |
| `systemdServices` | array of strings | [] | List of systemd service names to monitor |
| `services` | array of strings | [] | **Deprecated:** Use `dockerServices` instead. Automatically migrated on load. |

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
