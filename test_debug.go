package main

import (
"fmt"
"testing"
)

func TestDebugExtractWorkingDir(t *testing.T) {
labels := "com.docker.compose.project.working_dir=/myproject"
result := extractWorkingDir(labels)
fmt.Printf("Input: %s\n", labels)
fmt.Printf("Result: '%s'\n", result)
fmt.Printf("Length: %d\n", len(result))
}
