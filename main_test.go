package main

import (
	"testing"
)

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
