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
	Port            int      `json:"port"`
	Services        []string `json:"services,omitempty"`        // Deprecated: use DockerServices
	DockerServices  []string `json:"dockerServices,omitempty"`
	SystemdServices []string `json:"systemdServices,omitempty"`
}

// DockerContainer represents a Docker container from `docker ps --format json`
type DockerContainer struct {
	Labels string `json:"Labels"`
	Names  string `json:"Names"`
	Status string `json:"Status"`
}

// DockerStats represents container stats from `docker stats --format json --no-stream`
type DockerStats struct {
	Container string `json:"Container"`
	Name      string `json:"Name"`
	CPUPerc   string `json:"CPUPerc"`
	MemUsage  string `json:"MemUsage"`
	MemPerc   string `json:"MemPerc"`
	NetIO     string `json:"NetIO"`
	BlockIO   string `json:"BlockIO"`
	PIDs      string `json:"PIDs"`
}

// ServiceStatus represents the status of a service
type ServiceStatus struct {
	Name      string
	ShortName string
	Running   bool
	Status    string
	Type      string // "docker" or "systemd"
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
	http.HandleFunc("/ps", psHandler)
	http.HandleFunc("/stats", statsHandler)
	http.HandleFunc("/systemctl-is-active", systemctlIsActiveHandler)
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

	// Backward compatibility: migrate old "services" to "dockerServices"
	if len(config.Services) > 0 && len(config.DockerServices) == 0 {
		config.DockerServices = config.Services
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

	// Create a map of service names to container info
	runningMap := make(map[string]ContainerInfo)
	for _, info := range runningServices {
		runningMap[info.ServiceName] = info
	}

	var statuses []ServiceStatus
	
	// Add Docker services
	for _, expectedService := range config.DockerServices {
		// ShortName is the last segment after '/'
		shortName := expectedService
		if idx := strings.LastIndex(expectedService, "/"); idx != -1 && idx+1 < len(expectedService) {
			shortName = expectedService[idx+1:]
		}
		status := ServiceStatus{
			Name:      expectedService,
			ShortName: shortName,
			Running:   false,
			Type:      "docker",
		}

		if containerInfo, exists := runningMap[expectedService]; exists {
			status.Running = true
			status.Status = containerInfo.Status
		}

		statuses = append(statuses, status)
	}
	
	// Add systemd services
	if len(config.SystemdServices) > 0 {
		systemdStatuses, err := getSystemdStatuses(config.SystemdServices)
		if err != nil {
			log.Printf("Warning: failed to get systemd statuses: %v", err)
		} else {
			statuses = append(statuses, systemdStatuses...)
		}
	}

	return statuses, nil
}

// getSystemdStatuses checks the status of systemd services
func getSystemdStatuses(services []string) ([]ServiceStatus, error) {
	if len(services) == 0 {
		return nil, nil
	}

	// Use systemctl is-active to check all services at once
	args := append([]string{"is-active"}, services...)
	cmd := exec.Command("systemctl", args...)
	output, err := cmd.Output()
	
	// systemctl is-active returns non-zero exit code if any service is not active
	// but still provides output for each service, so we ignore the error
	
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var statuses []ServiceStatus
	
	for i, service := range services {
		status := ServiceStatus{
			Name:      service,
			ShortName: service,
			Type:      "systemd",
			Running:   false,
		}
		
		if i < len(lines) {
			state := strings.TrimSpace(lines[i])
			status.Status = state
			status.Running = state == "active"
		}
		
		statuses = append(statuses, status)
	}
	
	return statuses, err
}

func psHandler(w http.ResponseWriter, r *http.Request) {
	// Execute docker ps --format json
	cmd := exec.Command("docker", "ps", "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			http.Error(w, fmt.Sprintf("docker ps failed: %s", string(exitErr.Stderr)), http.StatusInternalServerError)
			return
		}
		http.Error(w, fmt.Sprintf("docker ps failed: %v (is Docker installed and running?)", err), http.StatusInternalServerError)
		return
	}

	// Set content type to application/json
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	// Write the raw output from docker ps
	if _, err := w.Write(output); err != nil {
		log.Printf("Error writing response: %v", err)
	}
}

func systemctlIsActiveHandler(w http.ResponseWriter, r *http.Request) {
	// Parse query parameter for services
	servicesParam := r.URL.Query().Get("services")
	if servicesParam == "" {
		http.Error(w, "Missing 'services' query parameter", http.StatusBadRequest)
		return
	}
	
	services := strings.Split(servicesParam, ",")
	for i, s := range services {
		services[i] = strings.TrimSpace(s)
	}
	
	// Use systemctl is-active to check all services
	args := append([]string{"is-active"}, services...)
	cmd := exec.Command("systemctl", args...)
	output, _ := cmd.Output()
	
	// Parse output - one line per service
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	result := make(map[string]string)
	
	for i, service := range services {
		if i < len(lines) {
			result[service] = strings.TrimSpace(lines[i])
		} else {
			result[service] = "unknown"
		}
	}
	
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func statsHandler(w http.ResponseWriter, r *http.Request) {
	// Get running services
	runningServices, err := getRunningServices()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting running services: %v", err), http.StatusInternalServerError)
		return
	}

	// Get stats for all running containers
	statsMap, err := getContainerStats()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting container stats: %v", err), http.StatusInternalServerError)
		return
	}

	// Create a map of service names to container info
	runningMap := make(map[string]ContainerInfo)
	for _, info := range runningServices {
		runningMap[info.ServiceName] = info
	}

	// Build response with stats for configured services
	type ServiceStats struct {
		Name     string `json:"name"`
		Running  bool   `json:"running"`
		CPUPerc  string `json:"cpuPerc,omitempty"`
		MemUsage string `json:"memUsage,omitempty"`
	}

	var response []ServiceStats
	for _, expectedService := range config.DockerServices {
		stat := ServiceStats{
			Name:    expectedService,
			Running: false,
		}

		if containerInfo, exists := runningMap[expectedService]; exists {
			stat.Running = true
			// Get stats for this container
			if stats, hasStats := statsMap[containerInfo.ContainerName]; hasStats {
				stat.CPUPerc = stats.CPUPerc
				stat.MemUsage = stats.MemUsage
			}
		}

		response = append(response, stat)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
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
    <title>Service Status Dashboard</title>
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

        .toggle-container {
            text-align: center;
            margin-bottom: 20px;
        }

        .toggle-button {
            background: white;
            border: none;
            border-radius: 5px;
            padding: 10px 20px;
            font-size: 0.9rem;
            font-weight: 600;
            cursor: pointer;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
            transition: all 0.2s;
            color: #374151;
        }

        .toggle-button:hover {
            transform: translateY(-2px);
            box-shadow: 0 4px 8px rgba(0,0,0,0.15);
        }

        .toggle-button.active {
            background: #10b981;
            color: white;
        }

        .toggle-button:disabled {
            opacity: 0.6;
            cursor: not-allowed;
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

        .service-type {
            display: inline-block;
            font-size: 0.7rem;
            padding: 2px 8px;
            border-radius: 3px;
            margin-top: 6px;
            font-weight: 500;
        }

        .service-type.docker {
            background: #2563eb;
            color: white;
        }

        .service-type.systemd {
            background: #059669;
            color: white;
        }

        .resource-stats {
            margin-top: 12px;
            padding-top: 12px;
            border-top: 1px solid #e5e7eb;
            display: none;
            grid-template-columns: 1fr 1fr;
            gap: 8px;
        }

        .resource-stats.visible {
            display: grid;
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
        <h1>{{.Icon}} Service Status Dashboard</h1>
        
        <div class="refresh-info">
            Auto-refresh: Reload the page to update status
        </div>

        <div class="toggle-container">
            <button id="statsToggle" class="toggle-button" onclick="toggleStats()">
                Show Resource Stats
            </button>
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
            <div class="service-card" data-service-name="{{.Name}}" data-service-type="{{.Type}}">
                <div class="service-header">
                    <div class="service-name">{{.ShortName}}</div>
                    <div class="status-indicator {{if .Running}}running{{else}}stopped{{end}}"></div>
                </div>
                <div class="status-text {{if .Running}}running{{else}}stopped{{end}}">
                    {{if .Running}}✓ Running{{else}}✗ Stopped{{end}}
                </div>
                <div class="service-type {{.Type}}">{{.Type}}</div>
                {{if .Status}}
                <div class="uptime-info">
                    {{.Status}}
                </div>
                {{end}}
                {{if and .Running (eq .Type "docker")}}
                <div class="resource-stats" data-service-name="{{.Name}}">
                    <div class="resource-item">
                        <div class="resource-label">CPU</div>
                        <div class="resource-value cpu-value">-</div>
                    </div>
                    <div class="resource-item">
                        <div class="resource-label">Memory</div>
                        <div class="resource-value mem-value">-</div>
                    </div>
                </div>
                {{end}}
            </div>
            {{end}}
        </div>
    </div>

    <script>
        let statsEnabled = false;

        async function toggleStats() {
            const button = document.getElementById('statsToggle');
            statsEnabled = !statsEnabled;

            if (statsEnabled) {
                button.classList.add('active');
                button.textContent = 'Hide Resource Stats';
                button.disabled = true;
                await loadStats();
                button.disabled = false;
            } else {
                button.classList.remove('active');
                button.textContent = 'Show Resource Stats';
                hideStats();
            }
        }

        async function loadStats() {
            try {
                const response = await fetch('/stats');
                if (!response.ok) {
                    throw new Error('Failed to fetch stats');
                }
                const stats = await response.json();

                stats.forEach(service => {
                    if (service.running) {
                        const card = document.querySelector('.service-card[data-service-name="' + service.name + '"]');
                        if (card) {
                            const statsDiv = card.querySelector('.resource-stats[data-service-name="' + service.name + '"]');
                            if (statsDiv) {
                                const cpuValue = statsDiv.querySelector('.cpu-value');
                                const memValue = statsDiv.querySelector('.mem-value');
                                
                                if (cpuValue) cpuValue.textContent = service.cpuPerc || '-';
                                if (memValue) memValue.textContent = service.memUsage || '-';
                                
                                statsDiv.classList.add('visible');
                            }
                        }
                    }
                });
            } catch (error) {
                console.error('Error loading stats:', error);
                alert('Failed to load resource stats. Please try again.');
            }
        }

        function hideStats() {
            const allStats = document.querySelectorAll('.resource-stats');
            allStats.forEach(stats => {
                stats.classList.remove('visible');
            });
        }
    </script>
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
