package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		{"Too long", string(make([]byte, 257)), false},
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
