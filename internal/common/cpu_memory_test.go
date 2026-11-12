package common

import "testing"

func TestCPUMemoryConfigs_ValidateAndNormalize(t *testing.T) {
	testCases := []struct {
		name         string
		configs      CPUMemoryConfigs
		cpu          string
		memoryGbs    string
		expectError  bool
		expectNil    bool
		expectShared bool
		expectedCPU  int
		expectedMem  int
	}{
		{
			name:      "Neither set (returns nil)",
			configs:   StandardCPUMemoryConfigs(),
			expectNil: true,
		},
		{
			name:        "Standard both set (1 CPU, 4GB)",
			configs:     StandardCPUMemoryConfigs(),
			cpu:         "1",
			memoryGbs:   "4",
			expectedCPU: 1000,
			expectedMem: 4,
		},
		{
			name:        "Standard fractional cores (0.5 CPU, 2GB)",
			configs:     StandardCPUMemoryConfigs(),
			cpu:         "0.5",
			memoryGbs:   "2",
			expectedCPU: 500,
			expectedMem: 2,
		},
		{
			name:        "Standard CPU only auto-configures memory (2 CPU -> 8GB)",
			configs:     StandardCPUMemoryConfigs(),
			cpu:         "2",
			expectedCPU: 2000,
			expectedMem: 8,
		},
		{
			name:        "Standard memory only auto-configures CPU (16GB -> 4 CPU)",
			configs:     StandardCPUMemoryConfigs(),
			memoryGbs:   "16",
			expectedCPU: 4000,
			expectedMem: 16,
		},
		{
			name:        "Standard deprecated millicores (1000m, 4GB)",
			configs:     StandardCPUMemoryConfigs(),
			cpu:         "1000",
			memoryGbs:   "4",
			expectedCPU: 1000,
			expectedMem: 4,
		},
		{
			name:         "Standard shared/shared",
			configs:      StandardCPUMemoryConfigs(),
			cpu:          "shared",
			memoryGbs:    "shared",
			expectShared: true,
		},
		{
			name:         "Standard CPU shared only auto-configures memory to shared",
			configs:      StandardCPUMemoryConfigs(),
			cpu:          "shared",
			expectShared: true,
		},
		{
			name:         "Standard memory shared only auto-configures CPU to shared",
			configs:      StandardCPUMemoryConfigs(),
			memoryGbs:    "shared",
			expectShared: true,
		},
		{
			name:        "Standard rejects a mismatched combination (1 CPU, 8GB)",
			configs:     StandardCPUMemoryConfigs(),
			cpu:         "1",
			memoryGbs:   "8",
			expectError: true,
		},
		{
			name:        "Standard rejects invalid cores (1.5 CPU)",
			configs:     StandardCPUMemoryConfigs(),
			cpu:         "1.5",
			expectError: true,
		},
		{
			name:        "Standard rejects invalid millicores (1500m)",
			configs:     StandardCPUMemoryConfigs(),
			cpu:         "1500",
			expectError: true,
		},
		{
			name:        "Standard rejects invalid memory (12GB)",
			configs:     StandardCPUMemoryConfigs(),
			memoryGbs:   "12",
			expectError: true,
		},
		{
			name:        "Standard rejects non-numeric CPU",
			configs:     StandardCPUMemoryConfigs(),
			cpu:         "two",
			expectError: true,
		},
		{
			name:        "Standard rejects non-numeric memory",
			configs:     StandardCPUMemoryConfigs(),
			memoryGbs:   "eight",
			expectError: true,
		},
		{
			name:        "Fluid both set (1 CPU, 8GB)",
			configs:     FluidCPUMemoryConfigs(),
			cpu:         "1",
			memoryGbs:   "8",
			expectedCPU: 1000,
			expectedMem: 8,
		},
		{
			name:        "Fluid fractional cores (0.25 CPU, 2GB)",
			configs:     FluidCPUMemoryConfigs(),
			cpu:         "0.25",
			memoryGbs:   "2",
			expectedCPU: 250,
			expectedMem: 2,
		},
		{
			name:        "Fluid CPU only auto-configures memory (1 CPU -> 8GB)",
			configs:     FluidCPUMemoryConfigs(),
			cpu:         "1",
			expectedCPU: 1000,
			expectedMem: 8,
		},
		{
			name:        "Fluid memory only auto-configures CPU (16GB -> 2 CPU)",
			configs:     FluidCPUMemoryConfigs(),
			memoryGbs:   "16",
			expectedCPU: 2000,
			expectedMem: 16,
		},
		{
			name:        "Fluid memory only auto-configures fractional CPU (2GB -> 0.25 CPU)",
			configs:     FluidCPUMemoryConfigs(),
			memoryGbs:   "2",
			expectedCPU: 250,
			expectedMem: 2,
		},
		{
			name:        "Fluid deprecated millicores (250m, 2GB)",
			configs:     FluidCPUMemoryConfigs(),
			cpu:         "250",
			memoryGbs:   "2",
			expectedCPU: 250,
			expectedMem: 2,
		},
		{
			name:        "Fluid rejects a standard combination (1 CPU, 4GB)",
			configs:     FluidCPUMemoryConfigs(),
			cpu:         "1",
			memoryGbs:   "4",
			expectError: true,
		},
		{
			name:        "Fluid rejects shared",
			configs:     FluidCPUMemoryConfigs(),
			cpu:         "shared",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config, err := tc.configs.ValidateAndNormalize(tc.cpu, tc.memoryGbs)

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
				if config != nil {
					t.Errorf("Expected nil config, got %+v", config)
				}
				return
			}

			if config == nil {
				t.Errorf("Expected non-nil config, got nil")
				return
			}

			if tc.expectShared {
				if !config.Shared {
					t.Errorf("Expected shared config, got %+v", config)
				}
				return
			}

			if config.CPUMillis != tc.expectedCPU {
				t.Errorf("Expected CPU %d, got %d", tc.expectedCPU, config.CPUMillis)
			}

			if config.MemoryGBs != tc.expectedMem {
				t.Errorf("Expected memory %d, got %d", tc.expectedMem, config.MemoryGBs)
			}
		})
	}
}

// TestCPUMemoryConfigs_CoresAndMillicoresDisjoint guards the assumption that
// lets Matches accept CPU in either cores or millicores: no configuration's
// core count equals any configuration's millicore count.
func TestCPUMemoryConfigs_CoresAndMillicoresDisjoint(t *testing.T) {
	configs := append(StandardCPUMemoryConfigs(), FluidCPUMemoryConfigs()...).WithoutShared()
	for _, a := range configs {
		for _, b := range configs {
			if float64(a.CPUMillis)/1000 == float64(b.CPUMillis) {
				t.Errorf("%s CPU is ambiguous with %dm", FormatCPUCores(a.CPUMillis), b.CPUMillis)
			}
		}
	}
}

func TestStandardCPUMemoryConfigs(t *testing.T) {
	configs := StandardCPUMemoryConfigs()

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
		{CPUMillis: 250, MemoryGBs: 2},
		{CPUMillis: 500, MemoryGBs: 2},
		{CPUMillis: 1000, MemoryGBs: 4},
		{CPUMillis: 2000, MemoryGBs: 8},
	}

	result := configs.String()
	expected := "shared/shared, 0.25 CPU/2 GB, 0.5 CPU/2 GB, 1 CPU/4 GB, 2 CPU/8 GB"

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}
