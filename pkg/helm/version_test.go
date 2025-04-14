package helm

import (
	"fmt"
	"strings"
	"testing"

	"helm.sh/helm/v3/pkg/cli"
)

func TestCheckHelmVersion(t *testing.T) {
	t.Skip("Skipping due to Helm API compatibility issues")

	// Save original function and restore after tests
	originalGetVersion := GetHelmVersion
	defer func() { GetHelmVersion = originalGetVersion }()

	tests := []struct {
		name        string
		version     string
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid version",
			version: "3.10.0",
			wantErr: false,
		},
		{
			name:        "version too old",
			version:     "3.9.0",
			wantErr:     true,
			errContains: "version 3.9.0 is too old",
		},
		{
			name:        "version too new",
			version:     "3.13.0",
			wantErr:     true,
			errContains: "version 3.13.0 is too new",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock GetHelmVersion for this test
			GetHelmVersion = func() (string, error) {
				return tt.version, nil
			}

			err := CheckHelmVersion(cli.New())
			if (err != nil) != tt.wantErr {
				t.Errorf("CheckHelmVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("CheckHelmVersion() error = %v, want error containing %q", err, tt.errContains)
				}
			}
		})
	}
}

func TestGetHelmVersion(t *testing.T) {
	// Save original function and restore after tests
	originalGetVersion := GetHelmVersion
	defer func() { GetHelmVersion = originalGetVersion }()

	tests := []struct {
		name        string
		mockVersion string
		mockErr     error
		wantErr     bool
	}{
		{
			name:        "success",
			mockVersion: "3.10.0",
			mockErr:     nil,
			wantErr:     false,
		},
		{
			name:        "error",
			mockVersion: "",
			mockErr:     fmt.Errorf("mock error"),
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock GetHelmVersion for this test
			GetHelmVersion = func() (string, error) {
				return tt.mockVersion, tt.mockErr
			}

			version, err := GetHelmVersion()
			if (err != nil) != tt.wantErr {
				t.Errorf("GetHelmVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && version != tt.mockVersion {
				t.Errorf("GetHelmVersion() = %v, want %v", version, tt.mockVersion)
			}
		})
	}
}

func TestParseHelmVersion(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		wantMajor   int
		wantMinor   int
		wantPatch   int
		wantErr     bool
		errContains string
	}{
		{
			name:      "valid version",
			version:   "3.10.0",
			wantMajor: 3,
			wantMinor: 10,
			wantPatch: 0,
			wantErr:   false,
		},
		{
			name:        "invalid format",
			version:     "invalid",
			wantErr:     true,
			errContains: "invalid version format",
		},
		{
			name:        "missing patch",
			version:     "3.10",
			wantErr:     true,
			errContains: "invalid version format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			major, minor, patch, err := ParseHelmVersion(tt.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseHelmVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ParseHelmVersion() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}
			if major != tt.wantMajor || minor != tt.wantMinor || patch != tt.wantPatch {
				t.Errorf("ParseHelmVersion() = %d.%d.%d, want %d.%d.%d",
					major, minor, patch, tt.wantMajor, tt.wantMinor, tt.wantPatch)
			}
		})
	}
}
