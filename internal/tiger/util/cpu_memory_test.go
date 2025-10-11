package util

import "testing"

func TestValidateAndNormalizeCPUMemory(t *testing.T) {
	testCases := []struct {
		name          string
		cpuMillis     int
		memoryGbs     int
		cpuFlagSet    bool
		memoryFlagSet bool
		expectError   bool
		expectedCPU   int
		expectedMem   int
	}{
		{
			name:          "Valid combination both set (1 CPU, 4GB)",
			cpuMillis:     1000,
			memoryGbs:     4,
			cpuFlagSet:    true,
			memoryFlagSet: true,
			expectError:   false,
			expectedCPU:   1000,
			expectedMem:   4,
		},
		{
			name:          "Valid combination both set (0.5 CPU, 2GB)",
			cpuMillis:     500,
			memoryGbs:     2,
			cpuFlagSet:    true,
			memoryFlagSet: true,
			expectError:   false,
			expectedCPU:   500,
			expectedMem:   2,
		},
		{
			name:          "Invalid combination both set (1 CPU, 8GB)",
			cpuMillis:     1000,
			memoryGbs:     8,
			cpuFlagSet:    true,
			memoryFlagSet: true,
			expectError:   true,
		},
		{
			name:          "CPU only auto-configure memory (2 CPU -> 8GB)",
			cpuMillis:     2000,
			memoryGbs:     0, // ignored
			cpuFlagSet:    true,
			memoryFlagSet: false,
			expectError:   false,
			expectedCPU:   2000,
			expectedMem:   8,
		},
		{
			name:          "Memory only auto-configure CPU (16GB -> 4 CPU)",
			cpuMillis:     0, // ignored
			memoryGbs:     16,
			cpuFlagSet:    false,
			memoryFlagSet: true,
			expectError:   false,
			expectedCPU:   4000,
			expectedMem:   16,
		},
		{
			name:          "Invalid CPU only",
			cpuMillis:     1500, // not in allowed configs
			memoryGbs:     0,    // ignored
			cpuFlagSet:    true,
			memoryFlagSet: false,
			expectError:   true,
		},
		{
			name:          "Invalid memory only",
			cpuMillis:     0,  // ignored
			memoryGbs:     12, // not in allowed configs
			cpuFlagSet:    false,
			memoryFlagSet: true,
			expectError:   true,
		},
		{
			name:          "Neither flag set (use defaults)",
			cpuMillis:     0,
			memoryGbs:     0,
			cpuFlagSet:    false,
			memoryFlagSet: false,
			expectError:   false,
			expectedCPU:   500,
			expectedMem:   2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cpu, memory, err := ValidateAndNormalizeCPUMemory(tc.cpuMillis, tc.memoryGbs, tc.cpuFlagSet, tc.memoryFlagSet)

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if cpu != tc.expectedCPU {
				t.Errorf("Expected CPU %d, got %d", tc.expectedCPU, cpu)
			}

			if memory != tc.expectedMem {
				t.Errorf("Expected memory %d, got %d", tc.expectedMem, memory)
			}
		})
	}
}

func TestGetAllowedCPUMemoryConfigs(t *testing.T) {
	configs := GetAllowedCPUMemoryConfigs()

	// Verify we have the expected number of configurations
	expectedCount := 7
	if len(configs) != expectedCount {
		t.Errorf("Expected %d configurations, got %d", expectedCount, len(configs))
	}

	// Verify specific configurations from the spec
	expectedConfigs := []CPUMemoryConfig{
		{CPUMillis: 500, MemoryGBs: 2},
		{CPUMillis: 1000, MemoryGBs: 4},
		{CPUMillis: 2000, MemoryGBs: 8},
		{CPUMillis: 4000, MemoryGBs: 16},
		{CPUMillis: 8000, MemoryGBs: 32},
		{CPUMillis: 16000, MemoryGBs: 64},
		{CPUMillis: 32000, MemoryGBs: 128},
	}

	for i, expected := range expectedConfigs {
		if i < len(configs) {
			if configs[i].CPUMillis != expected.CPUMillis || configs[i].MemoryGBs != expected.MemoryGBs {
				t.Errorf("Config %d: expected %+v, got %+v", i, expected, configs[i])
			}
		}
	}
}

func TestCPUMemoryConfigs_String(t *testing.T) {
	configs := CPUMemoryConfigs{
		{CPUMillis: 500, MemoryGBs: 2},
		{CPUMillis: 1000, MemoryGBs: 4},
		{CPUMillis: 2000, MemoryGBs: 8},
	}

	result := configs.String()
	expected := "0.5 CPU/2GB, 1 CPU/4GB, 2 CPU/8GB"

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestCPUMemoryConfigs_CPUString(t *testing.T) {
	configs := CPUMemoryConfigs{
		{CPUMillis: 500, MemoryGBs: 2},
		{CPUMillis: 1000, MemoryGBs: 4},
		{CPUMillis: 2000, MemoryGBs: 8},
	}

	result := configs.CPUString()
	expected := "0.5 (500m), 1 (1000m), 2 (2000m)"

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestCPUMemoryConfigs_MemoryString(t *testing.T) {
	configs := CPUMemoryConfigs{
		{CPUMillis: 500, MemoryGBs: 2},
		{CPUMillis: 1000, MemoryGBs: 4},
		{CPUMillis: 2000, MemoryGBs: 8},
	}

	result := configs.MemoryString()
	expected := "2GB, 4GB, 8GB"

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}
