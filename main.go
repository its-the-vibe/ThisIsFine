package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

// Config represents the application configuration
type Config struct {
	Port     int      `json:"port"`
	Services []string `json:"services"`
}

// DockerContainer represents a Docker container from `docker ps --format json`
type DockerContainer struct {
	Labels string `json:"Labels"`
	Names  string `json:"Names"`
	Status string `json:"Status"`
}

// DockerStats represents container stats from `docker stats --format json --no-stream`
type DockerStats struct {
	Container   string `json:"Container"`
	Name        string `json:"Name"`
	CPUPerc     string `json:"CPUPerc"`
	MemUsage    string `json:"MemUsage"`
	MemPerc     string `json:"MemPerc"`
	NetIO       string `json:"NetIO"`
	BlockIO     string `json:"BlockIO"`
	PIDs        string `json:"PIDs"`
}

// ServiceStatus represents the status of a service
type ServiceStatus struct {
	Name     string
	Running  bool
	CPUPerc  string
	MemUsage string
	Status   string
}

var config Config

func main() {
	configPath := flag.String("config", "config.json", "Path to configuration file")
	flag.Parse()

	// Load configuration
	if err := loadConfig(*configPath); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Set up HTTP handlers
	http.HandleFunc("/", statusHandler)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Start server
	addr := fmt.Sprintf(":%d", config.Port)
	log.Printf("Starting server on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func loadConfig(path string) error {
	file, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading config file: %w", err)
	}

	if err := json.Unmarshal(file, &config); err != nil {
		return fmt.Errorf("parsing config file: %w", err)
	}

	if config.Port == 0 {
		config.Port = 8080
	}

	return nil
}

// ContainerInfo holds both service name and container name
type ContainerInfo struct {
	ServiceName   string
	ContainerName string
	Status        string
}

func getRunningServices() ([]ContainerInfo, error) {
	cmd := exec.Command("docker", "ps", "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("executing docker ps: %w (stderr: %s)", err, string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("executing docker ps: %w (is Docker installed and running?)", err)
	}

	var runningServices []ContainerInfo
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		var container DockerContainer
		if err := json.Unmarshal([]byte(line), &container); err != nil {
			log.Printf("Warning: failed to parse container JSON: %v", err)
			continue
		}

		// Extract working_dir from labels
		workingDir := extractWorkingDir(container.Labels)
		if workingDir != "" {
			runningServices = append(runningServices, ContainerInfo{
				ServiceName:   workingDir,
				ContainerName: container.Names,
				Status:        container.Status,
			})
		}
	}

	return runningServices, nil
}

func extractWorkingDir(labels string) string {
	// Parse labels to find com.docker.compose.project.working_dir
	labelPairs := strings.Split(labels, ",")
	for _, pair := range labelPairs {
		if strings.HasPrefix(pair, "com.docker.compose.project.working_dir=") {
			fullPath := strings.TrimPrefix(pair, "com.docker.compose.project.working_dir=")
			// Extract only the last 2 parts (orgname/servicename)
			fullPath = strings.TrimSuffix(fullPath, "/")
			parts := strings.Split(fullPath, "/")
			// Filter out empty parts
			var nonEmptyParts []string
			for _, part := range parts {
				if part != "" {
					nonEmptyParts = append(nonEmptyParts, part)
				}
			}
			if len(nonEmptyParts) >= 2 {
				return nonEmptyParts[len(nonEmptyParts)-2] + "/" + nonEmptyParts[len(nonEmptyParts)-1]
			}
			// If less than 2 parts, return the full path as-is
			return fullPath
		}
	}
	return ""
}

// getContainerStats fetches resource usage statistics for all running containers
// using `docker stats --format json --no-stream`. Returns a map of container stats
// keyed by container name.
func getContainerStats() (map[string]DockerStats, error) {
	cmd := exec.Command("docker", "stats", "--format", "json", "--no-stream")
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("executing docker stats: %w (stderr: %s)", err, string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("executing docker stats: %w", err)
	}

	statsMap := make(map[string]DockerStats)
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		var stats DockerStats
		if err := json.Unmarshal([]byte(line), &stats); err != nil {
			log.Printf("Warning: failed to parse stats JSON: %v", err)
			continue
		}

		// Store stats by container name
		statsMap[stats.Name] = stats
	}

	return statsMap, nil
}

func getServiceStatuses() ([]ServiceStatus, error) {
	runningServices, err := getRunningServices()
	if err != nil {
		return nil, err
	}

	// Get stats for all running containers
	statsMap, err := getContainerStats()
	if err != nil {
		log.Printf("Warning: failed to get container stats: %v", err)
		// Continue without stats rather than failing completely
		statsMap = make(map[string]DockerStats)
	}

	// Create a map of service names to container info
	runningMap := make(map[string]ContainerInfo)
	for _, info := range runningServices {
		runningMap[info.ServiceName] = info
	}

	var statuses []ServiceStatus
	for _, expectedService := range config.Services {
		status := ServiceStatus{
			Name:    expectedService,
			Running: false,
		}

		if containerInfo, exists := runningMap[expectedService]; exists {
			status.Running = true
			status.Status = containerInfo.Status
			// Get stats for this container
			if stats, hasStats := statsMap[containerInfo.ContainerName]; hasStats {
				status.CPUPerc = stats.CPUPerc
				status.MemUsage = stats.MemUsage
			}
		}

		statuses = append(statuses, status)
	}

	return statuses, nil
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	statuses, err := getServiceStatuses()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting service statuses: %v", err), http.StatusInternalServerError)
		return
	}

	tmpl := template.Must(template.New("status").Parse(`
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Docker Service Status Dashboard</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }

        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            {{if gt .StoppedCount 0}}
            background: #ED213A;  /* fallback for old browsers */
            background: -webkit-linear-gradient(to right, #93291E, #ED213A);  /* Chrome 10-25, Safari 5.1-6 */
            background: linear-gradient(to right, #93291E, #ED213A); /* W3C, IE 10+/ Edge, Firefox 16+, Chrome 26+, Opera 12+, Safari 7+ */
            {{else}}
            background: #83a4d4;  /* fallback for old browsers */
            background: -webkit-linear-gradient(to top, #b6fbff, #83a4d4);  /* Chrome 10-25, Safari 5.1-6 */
            background: linear-gradient(to top, #b6fbff, #83a4d4); /* W3C, IE 10+/ Edge, Firefox 16+, Chrome 26+, Opera 12+, Safari 7+ */
            {{end}}
            min-height: 100vh;
            padding: 20px;
        }

        .container {
            max-width: 1200px;
            margin: 0 auto;
        }

        h1 {
            color: white;
            text-align: center;
            margin-bottom: 30px;
            font-size: 2.5rem;
            text-shadow: 2px 2px 4px rgba(0,0,0,0.2);
        }

        .refresh-info {
            text-align: center;
            color: rgba(255, 255, 255, 0.9);
            margin-bottom: 20px;
            font-size: 0.9rem;
        }

        .services-grid {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
            gap: 20px;
            margin-bottom: 20px;
        }

        .service-card {
            background: white;
            border-radius: 10px;
            padding: 20px;
            box-shadow: 0 4px 6px rgba(0,0,0,0.1);
            transition: transform 0.2s, box-shadow 0.2s;
        }

        .service-card:hover {
            transform: translateY(-5px);
            box-shadow: 0 8px 12px rgba(0,0,0,0.15);
        }

        .service-header {
            display: flex;
            align-items: center;
            justify-content: space-between;
            margin-bottom: 10px;
        }

        .service-name {
            font-size: 1.1rem;
            font-weight: 600;
            color: #333;
            word-break: break-all;
        }

        .status-indicator {
            width: 12px;
            height: 12px;
            border-radius: 50%;
            flex-shrink: 0;
            margin-left: 10px;
        }

        .status-indicator.running {
            background: #10b981;
            box-shadow: 0 0 10px rgba(16, 185, 129, 0.5);
        }

        .status-indicator.stopped {
            background: #ef4444;
            box-shadow: 0 0 10px rgba(239, 68, 68, 0.5);
        }

        .status-text {
            font-size: 0.9rem;
            font-weight: 500;
            margin-top: 8px;
        }

        .status-text.running {
            color: #10b981;
        }

        .status-text.stopped {
            color: #ef4444;
        }

        .uptime-info {
            font-size: 0.85rem;
            color: #6b7280;
            margin-top: 4px;
        }

        .resource-stats {
            margin-top: 12px;
            padding-top: 12px;
            border-top: 1px solid #e5e7eb;
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 8px;
        }

        .resource-item {
            display: flex;
            flex-direction: column;
        }

        .resource-label {
            font-size: 0.75rem;
            color: #6b7280;
            margin-bottom: 2px;
        }

        .resource-value {
            font-size: 0.9rem;
            font-weight: 600;
            color: #374151;
        }

        .summary {
            background: white;
            border-radius: 10px;
            padding: 20px;
            text-align: center;
            box-shadow: 0 4px 6px rgba(0,0,0,0.1);
            margin-bottom: 20px;
        }

        .summary-stats {
            display: flex;
            justify-content: center;
            gap: 40px;
            margin-top: 15px;
        }

        .stat {
            display: flex;
            flex-direction: column;
            align-items: center;
        }

        .stat-value {
            font-size: 2rem;
            font-weight: 700;
        }

        .stat-value.running {
            color: #10b981;
        }

        .stat-value.stopped {
            color: #ef4444;
        }

        .stat-label {
            font-size: 0.9rem;
            color: #666;
            margin-top: 5px;
        }

        @media (max-width: 768px) {
            .services-grid {
                grid-template-columns: 1fr;
            }

            h1 {
                font-size: 2rem;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>{{.Icon}} Docker Service Status Dashboard</h1>
        
        <div class="refresh-info">
            Auto-refresh: Reload the page to update status
        </div>

        <div class="summary">
            <h2>Summary</h2>
            <div class="summary-stats">
                <div class="stat">
                    <div class="stat-value running">{{.RunningCount}}</div>
                    <div class="stat-label">Running</div>
                </div>
                <div class="stat">
                    <div class="stat-value stopped">{{.StoppedCount}}</div>
                    <div class="stat-label">Stopped</div>
                </div>
                <div class="stat">
                    <div class="stat-value">{{.TotalCount}}</div>
                    <div class="stat-label">Total</div>
                </div>
            </div>
        </div>

        <div class="services-grid">
            {{range .Services}}
            <div class="service-card">
                <div class="service-header">
                    <div class="service-name">{{.Name}}</div>
                    <div class="status-indicator {{if .Running}}running{{else}}stopped{{end}}"></div>
                </div>
                <div class="status-text {{if .Running}}running{{else}}stopped{{end}}">
                    {{if .Running}}✓ Running{{else}}✗ Stopped{{end}}
                </div>
                {{if .Status}}
                <div class="uptime-info">
                    {{.Status}}
                </div>
                {{end}}
                {{if .Running}}
                {{if or .CPUPerc .MemUsage}}
                <div class="resource-stats">
                    {{if .CPUPerc}}
                    <div class="resource-item">
                        <div class="resource-label">CPU</div>
                        <div class="resource-value">{{.CPUPerc}}</div>
                    </div>
                    {{end}}
                    {{if .MemUsage}}
                    <div class="resource-item">
                        <div class="resource-label">Memory</div>
                        <div class="resource-value">{{.MemUsage}}</div>
                    </div>
                    {{end}}
                </div>
                {{end}}
                {{end}}
            </div>
            {{end}}
        </div>
    </div>
</body>
</html>
`))

	runningCount := 0
	stoppedCount := 0
	for _, status := range statuses {
		if status.Running {
			runningCount++
		} else {
			stoppedCount++
		}
	}

	data := struct {
		Services     []ServiceStatus
		RunningCount int
		StoppedCount int
		TotalCount   int
		Icon         string
	}{
		Services:     statuses,
		RunningCount: runningCount,
		StoppedCount: stoppedCount,
		TotalCount:   len(statuses),
		Icon:         "😺",
	}

	if stoppedCount > 0 {
		data.Icon = "😿"
	}

	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Error executing template: %v", err)
	}
}
