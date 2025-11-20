package common

import "testing"

func TestValidateAndNormalizeCPUMemory(t *testing.T) {
	testCases := []struct {
		name        string
		cpuMillis   string
		memoryGbs   string
		expectError bool
		expectNil   bool
		expectedCPU string
		expectedMem string
	}{
		{
			name:        "Valid combination both set (1 CPU, 4GB)",
			cpuMillis:   "1000",
			memoryGbs:   "4",
			expectError: false,
			expectedCPU: "1000",
			expectedMem: "4",
		},
		{
			name:        "Valid combination both set (0.5 CPU, 2GB)",
			cpuMillis:   "500",
			memoryGbs:   "2",
			expectError: false,
			expectedCPU: "500",
			expectedMem: "2",
		},
		{
			name:        "Valid shared/shared combination",
			cpuMillis:   "shared",
			memoryGbs:   "shared",
			expectError: false,
			expectedCPU: "shared",
			expectedMem: "shared",
		},
		{
			name:        "CPU shared, memory empty (auto-configure to shared)",
			cpuMillis:   "shared",
			memoryGbs:   "",
			expectError: false,
			expectedCPU: "shared",
			expectedMem: "shared",
		},
		{
			name:        "CPU empty, memory shared (auto-configure to shared)",
			cpuMillis:   "",
			memoryGbs:   "shared",
			expectError: false,
			expectedCPU: "shared",
			expectedMem: "shared",
		},
		{
			name:        "Invalid combination both set (1 CPU, 8GB)",
			cpuMillis:   "1000",
			memoryGbs:   "8",
			expectError: true,
		},
		{
			name:        "CPU only auto-configure memory (2 CPU -> 8GB)",
			cpuMillis:   "2000",
			memoryGbs:   "",
			expectError: false,
			expectedCPU: "2000",
			expectedMem: "8",
		},
		{
			name:        "Memory only auto-configure CPU (16GB -> 4 CPU)",
			cpuMillis:   "",
			memoryGbs:   "16",
			expectError: false,
			expectedCPU: "4000",
			expectedMem: "16",
		},
		{
			name:        "Invalid CPU only",
			cpuMillis:   "1500",
			memoryGbs:   "",
			expectError: true,
		},
		{
			name:        "Invalid memory only",
			cpuMillis:   "",
			memoryGbs:   "12",
			expectError: true,
		},
		{
			name:        "Neither flag set (returns nil)",
			cpuMillis:   "",
			memoryGbs:   "",
			expectError: false,
			expectNil:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cpu, memory, err := ValidateAndNormalizeCPUMemory(tc.cpuMillis, tc.memoryGbs)

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

			if tc.expectNil {
				if cpu != nil || memory != nil {
					t.Errorf("Expected nil pointers, got cpu=%v, memory=%v", cpu, memory)
				}
				return
			}

			if cpu == nil || memory == nil {
				t.Errorf("Expected non-nil pointers, got cpu=%v, memory=%v", cpu, memory)
				return
			}

			if *cpu != tc.expectedCPU {
				t.Errorf("Expected CPU %s, got %s", tc.expectedCPU, *cpu)
			}

			if *memory != tc.expectedMem {
				t.Errorf("Expected memory %s, got %s", tc.expectedMem, *memory)
			}
		})
	}
}

func TestGetAllowedCPUMemoryConfigs(t *testing.T) {
	configs := GetAllowedCPUMemoryConfigs()

	// Verify we have the expected number of configurations (including shared)
	expectedCount := 8
	if len(configs) != expectedCount {
		t.Errorf("Expected %d configurations, got %d", expectedCount, len(configs))
	}

	// Verify specific configurations from the spec
	expectedConfigs := []CPUMemoryConfig{
		{Shared: true},                     // shared/shared
		{CPUMillis: 500, MemoryGBs: 2},     // 0.5 CPU/2GB
		{CPUMillis: 1000, MemoryGBs: 4},    // 1 CPU/4GB
		{CPUMillis: 2000, MemoryGBs: 8},    // 2 CPU/8GB
		{CPUMillis: 4000, MemoryGBs: 16},   // 4 CPU/16GB
		{CPUMillis: 8000, MemoryGBs: 32},   // 8 CPU/32GB
		{CPUMillis: 16000, MemoryGBs: 64},  // 16 CPU/64GB
		{CPUMillis: 32000, MemoryGBs: 128}, // 32 CPU/128GB
	}

	for i, expected := range expectedConfigs {
		if i < len(configs) {
			if configs[i].Shared != expected.Shared ||
				configs[i].CPUMillis != expected.CPUMillis ||
				configs[i].MemoryGBs != expected.MemoryGBs {
				t.Errorf("Config %d: expected %+v, got %+v", i, expected, configs[i])
			}
		}
	}
}

func TestCPUMemoryConfigs_String(t *testing.T) {
	configs := CPUMemoryConfigs{
		{Shared: true},
		{CPUMillis: 500, MemoryGBs: 2},
		{CPUMillis: 1000, MemoryGBs: 4},
		{CPUMillis: 2000, MemoryGBs: 8},
	}

	result := configs.String()
	expected := "shared/shared, 0.5 CPU/2 GB, 1 CPU/4 GB, 2 CPU/8 GB"

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}
