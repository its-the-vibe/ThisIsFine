package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsValidServiceName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"Valid simple name", "nginx", true},
		{"Valid with hyphen", "my-service", true},
		{"Valid with underscore", "my_service", true},
		{"Valid with dot", "nginx.service", true},
		{"Valid with @", "user@.service", true},
		{"Valid with colon", "service:name", true},
		{"Valid with backslash", "service\\name", true},
		{"Empty string", "", false},
		{"With spaces", "my service", false},
		{"With semicolon", "service;rm -rf", false},
		{"With pipe", "service|echo", false},
		{"With ampersand", "service&echo", false},
		{"With dollar", "service$var", false},
		{"With backtick", "service`cmd`", false},
		{"With parenthesis", "service(cmd)", false},
		{"Too long", strings.Repeat("a", 257), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidServiceName(tt.input)
			if result != tt.expected {
				t.Errorf("isValidServiceName(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

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
		{
			name:     "Path with trailing slash",
			labels:   "com.docker.compose.project.working_dir=/home/ubuntu/github/its-the-vibe/service-1/",
			expected: "its-the-vibe/service-1",
		},
		{
			name:     "Multiple labels",
			labels:   "com.docker.compose.project=myproject,com.docker.compose.project.working_dir=/home/ubuntu/github/its-the-vibe/rate-my,com.docker.compose.service=web",
			expected: "its-the-vibe/rate-my",
		},
		{
			name:     "No working_dir label",
			labels:   "com.docker.compose.project=myproject,com.docker.compose.service=web",
			expected: "",
		},
		{
			name:     "Empty labels",
			labels:   "",
			expected: "",
		},
		{
			name:     "Single segment path (returned as-is)",
			labels:   "com.docker.compose.project.working_dir=/myproject",
			expected: "/myproject",
		},
		{
			name:     "Two segment path",
			labels:   "com.docker.compose.project.working_dir=/org/repo",
			expected: "org/repo",
		},
		{
			name:     "Different base path",
			labels:   "com.docker.compose.project.working_dir=/var/www/its-the-vibe/service-2",
			expected: "its-the-vibe/service-2",
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

func TestGetContainerStats(t *testing.T) {
	// This is an integration test that requires Docker to be running
	// We'll skip it if Docker is not available
	statsMap, err := getContainerStats()
	if err != nil {
		t.Skipf("Skipping test - Docker may not be available: %v", err)
	}
	// Verify the function returns a map (not nil)
	if statsMap == nil {
		t.Error("getContainerStats returned nil map")
	}
	// If we got here, Docker is available and the function returned a valid map
}

func TestGetSystemdStatuses(t *testing.T) {
	tests := []struct {
		name     string
		services []string
	}{
		{
			name:     "Empty service list",
			services: []string{},
		},
		{
			name:     "Single service",
			services: []string{"dbus"},
		},
		{
			name:     "Multiple services",
			services: []string{"dbus", "systemd-journald"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			statuses, err := getSystemdStatuses(tt.services)
			
			if len(tt.services) == 0 {
				// Empty list should return nil
				if statuses != nil {
					t.Errorf("Expected nil for empty service list, got %v", statuses)
				}
				return
			}
			
			// For non-empty lists, we should get statuses (even if systemctl fails)
			if len(statuses) != len(tt.services) {
				t.Errorf("Expected %d statuses, got %d", len(tt.services), len(statuses))
			}
			
			// Check that each status has the correct fields
			for i, status := range statuses {
				if status.Name != tt.services[i] {
					t.Errorf("Status %d: expected name %s, got %s", i, tt.services[i], status.Name)
				}
				if status.Type != "systemd" {
					t.Errorf("Status %d: expected type 'systemd', got '%s'", i, status.Type)
				}
				if status.Status == "" {
					t.Errorf("Status %d: status field is empty", i)
				}
			}
			
			// The function should always return nil error now
			if err != nil {
				t.Errorf("Expected nil error, got: %v", err)
			}
		})
	}
}

func TestSystemctlIsActiveHandler(t *testing.T) {
	tests := []struct {
		name           string
		queryString    string
		expectedStatus int
		checkResponse  bool
	}{
		{
			name:           "Missing services parameter",
			queryString:    "",
			expectedStatus: http.StatusBadRequest,
			checkResponse:  false,
		},
		{
			name:           "Single service",
			queryString:    "services=dbus",
			expectedStatus: http.StatusOK,
			checkResponse:  true,
		},
		{
			name:           "Multiple services",
			queryString:    "services=dbus,systemd-journald",
			expectedStatus: http.StatusOK,
			checkResponse:  true,
		},
		{
			name:           "Invalid service name with semicolon",
			queryString:    "services=nginx;rm+-rf",
			expectedStatus: http.StatusBadRequest,
			checkResponse:  false,
		},
		{
			name:           "Invalid service name with pipe",
			queryString:    "services=nginx|echo+hack",
			expectedStatus: http.StatusBadRequest,
			checkResponse:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", "/systemctl-is-active?"+tt.queryString, nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			handler := http.HandlerFunc(systemctlIsActiveHandler)
			handler.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v", status, tt.expectedStatus)
			}

			if tt.checkResponse && rr.Code == http.StatusOK {
				// Check content type
				if contentType := rr.Header().Get("Content-Type"); contentType != "application/json" {
					t.Errorf("handler returned wrong content type: got %v want %v", contentType, "application/json")
				}

				// Try to decode the JSON response
				var result map[string]string
				if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
					t.Errorf("Failed to decode JSON response: %v", err)
				}
				
				// Verify we got results for the requested services
				if len(result) == 0 {
					t.Error("Expected non-empty result map")
				}
			}
		})
	}
}

func TestPsHandler(t *testing.T) {
	// This is an integration test that requires Docker to be running
	// Create a test HTTP request
	req, err := http.NewRequest("GET", "/ps", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Create a ResponseRecorder to record the response
	rr := httptest.NewRecorder()

	// Call the handler directly
	handler := http.HandlerFunc(psHandler)
	handler.ServeHTTP(rr, req)

	// Check the status code
	if status := rr.Code; status != http.StatusOK {
		// If Docker is not available, we might get an error
		// Skip the test instead of failing
		if status == http.StatusInternalServerError {
			t.Skipf("Skipping test - Docker may not be available (status: %d)", status)
		}
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	// Check the content type
	if contentType := rr.Header().Get("Content-Type"); contentType != "application/json" {
		t.Errorf("handler returned wrong content type: got %v want %v", contentType, "application/json")
	}

	// Verify the response is valid (at least not empty, if Docker is running)
	// The response should be JSON lines format from docker ps
	if rr.Body.Len() == 0 {
		t.Log("Warning: Empty response from docker ps (no containers running)")
	}
}

func TestParseProcStatCPU(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantErr     bool
		wantUser    uint64
		wantNice    uint64
		wantSystem  uint64
		wantIdle    uint64
		wantIowait  uint64
		wantIrq     uint64
		wantSoftirq uint64
		wantSteal   uint64
	}{
		{
			name:        "Normal /proc/stat line",
			content:     "cpu  100 20 50 800 10 5 3 2 0 0\ncpu0 50 10 25 400 5 2 1 1 0 0\n",
			wantUser:    100,
			wantNice:    20,
			wantSystem:  50,
			wantIdle:    800,
			wantIowait:  10,
			wantIrq:     5,
			wantSoftirq: 3,
			wantSteal:   2,
		},
		{
			name:        "Minimal valid line",
			content:     "cpu  1 2 3 4 5 6 7 8\n",
			wantUser:    1,
			wantNice:    2,
			wantSystem:  3,
			wantIdle:    4,
			wantIowait:  5,
			wantIrq:     6,
			wantSoftirq: 7,
			wantSteal:   8,
		},
		{
			name:    "Missing cpu line",
			content: "cpuinfo some data\n",
			wantErr: true,
		},
		{
			name:    "Too few fields",
			content: "cpu  1 2 3 4\n",
			wantErr: true,
		},
		{
			name:    "Empty content",
			content: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseProcStatCPU(tt.content)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.user != tt.wantUser {
				t.Errorf("user = %d, want %d", got.user, tt.wantUser)
			}
			if got.nice != tt.wantNice {
				t.Errorf("nice = %d, want %d", got.nice, tt.wantNice)
			}
			if got.system != tt.wantSystem {
				t.Errorf("system = %d, want %d", got.system, tt.wantSystem)
			}
			if got.idle != tt.wantIdle {
				t.Errorf("idle = %d, want %d", got.idle, tt.wantIdle)
			}
			if got.iowait != tt.wantIowait {
				t.Errorf("iowait = %d, want %d", got.iowait, tt.wantIowait)
			}
		})
	}
}

func TestProcStatCPUTotals(t *testing.T) {
	c := procStatCPU{user: 100, nice: 20, system: 50, idle: 800, iowait: 10, irq: 5, softirq: 3, steal: 2}
	wantTotal := uint64(990)
	if got := c.total(); got != wantTotal {
		t.Errorf("total() = %d, want %d", got, wantTotal)
	}
	wantIdle := uint64(810)
	if got := c.idleTotal(); got != wantIdle {
		t.Errorf("idleTotal() = %d, want %d", got, wantIdle)
	}
}

func TestParseMemInfo(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantUsed  uint64
		wantTotal uint64
		wantErr   bool
	}{
		{
			name: "Normal /proc/meminfo",
			content: `MemTotal:       16384000 kB
MemFree:         1234567 kB
MemAvailable:    8192000 kB
Buffers:          123456 kB
Cached:          1234567 kB
`,
			wantTotal: 16384000 * 1024,
			wantUsed:  (16384000 - 8192000) * 1024,
		},
		{
			name: "Missing MemAvailable",
			content: `MemTotal:       16384000 kB
MemFree:         1234567 kB
`,
			wantErr: true,
		},
		{
			name: "Missing MemTotal",
			content: `MemFree:         1234567 kB
MemAvailable:    8192000 kB
`,
			wantErr: true,
		},
		{
			name:    "Empty content",
			content: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			used, total, err := parseMemInfo(tt.content)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if total != tt.wantTotal {
				t.Errorf("total = %d, want %d", total, tt.wantTotal)
			}
			if used != tt.wantUsed {
				t.Errorf("used = %d, want %d", used, tt.wantUsed)
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name     string
		input    uint64
		expected string
	}{
		{"Zero bytes", 0, "0 B"},
		{"Less than 1 KiB", 512, "512 B"},
		{"Exactly 1 KiB", 1024, "1.0 KiB"},
		{"1.5 KiB", 1536, "1.5 KiB"},
		{"1 MiB", 1024 * 1024, "1.0 MiB"},
		{"1.5 GiB", uint64(1.5 * 1024 * 1024 * 1024), "1.5 GiB"},
		{"1 TiB", 1024 * 1024 * 1024 * 1024, "1.0 TiB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatBytes(tt.input)
			if result != tt.expected {
				t.Errorf("formatBytes(%d) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSystemStatsHandler(t *testing.T) {
	req, err := http.NewRequest("GET", "/system-stats", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(systemStatsHandler)
	handler.ServeHTTP(rr, req)

	// On Linux the /proc filesystem should always be available.
	// If not (e.g. exotic CI), skip rather than fail.
	if rr.Code == http.StatusInternalServerError {
		t.Skipf("Skipping test - /proc may not be available: %s", rr.Body.String())
	}

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("wrong Content-Type: got %q, want %q", ct, "application/json")
	}

	var result SystemStats
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	if result.MemTotalBytes == 0 {
		t.Error("MemTotalBytes should be non-zero")
	}
	if result.CPUUsagePercent < 0 || result.CPUUsagePercent > 100 {
		t.Errorf("CPUUsagePercent %f out of range [0,100]", result.CPUUsagePercent)
	}
	if result.MemUsagePercent < 0 || result.MemUsagePercent > 100 {
		t.Errorf("MemUsagePercent %f out of range [0,100]", result.MemUsagePercent)
	}
}
