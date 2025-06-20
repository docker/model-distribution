package types

import (
	"testing"
)

func TestIOTypesValidate(t *testing.T) {
	tests := []struct {
		name    string
		io      IOTypes
		wantErr bool
	}{
		{
			name: "valid text model",
			io: IOTypes{
				Input:  []string{IOTypeText},
				Output: []string{IOTypeText},
			},
			wantErr: false,
		},
		{
			name: "valid multimodal model",
			io: IOTypes{
				Input:  []string{IOTypeText, IOTypeImage},
				Output: []string{IOTypeText},
			},
			wantErr: false,
		},
		{
			name: "invalid input type",
			io: IOTypes{
				Input:  []string{"invalid"},
				Output: []string{IOTypeText},
			},
			wantErr: true,
		},
		{
			name: "invalid output type",
			io: IOTypes{
				Input:  []string{IOTypeText},
				Output: []string{"invalid"},
			},
			wantErr: true,
		},
		{
			name: "duplicate input type",
			io: IOTypes{
				Input:  []string{IOTypeText, IOTypeText},
				Output: []string{IOTypeText},
			},
			wantErr: true,
		},
		{
			name: "duplicate output type",
			io: IOTypes{
				Input:  []string{IOTypeText},
				Output: []string{IOTypeText, IOTypeText},
			},
			wantErr: true,
		},
		{
			name: "empty input",
			io: IOTypes{
				Input:  []string{},
				Output: []string{IOTypeText},
			},
			wantErr: false,
		},
		{
			name: "empty output",
			io: IOTypes{
				Input:  []string{IOTypeText},
				Output: []string{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.io.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("IOTypes.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCapabilitiesValidate(t *testing.T) {
	tests := []struct {
		name         string
		capabilities *Capabilities
		wantErr      bool
	}{
		{
			name: "valid capabilities",
			capabilities: &Capabilities{
				IO: IOTypes{
					Input:  []string{IOTypeText},
					Output: []string{IOTypeText},
				},
				ToolUsage: boolPtr(true),
			},
			wantErr: false,
		},
		{
			name:         "nil capabilities",
			capabilities: nil,
			wantErr:      true,
		},
		{
			name: "invalid IO types",
			capabilities: &Capabilities{
				IO: IOTypes{
					Input:  []string{"invalid"},
					Output: []string{IOTypeText},
				},
				ToolUsage: boolPtr(true),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.capabilities.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Capabilities.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewCapabilities(t *testing.T) {
	tests := []struct {
		name      string
		input     []string
		output    []string
		toolUsage bool
		wantErr   bool
		wantNil   bool
	}{
		{
			name:      "valid capabilities",
			input:     []string{IOTypeText},
			output:    []string{IOTypeText},
			toolUsage: true,
			wantErr:   false,
			wantNil:   false,
		},
		{
			name:      "invalid input type",
			input:     []string{"invalid"},
			output:    []string{IOTypeText},
			toolUsage: true,
			wantErr:   true,
			wantNil:   true,
		},
		{
			name:      "invalid output type",
			input:     []string{IOTypeText},
			output:    []string{"invalid"},
			toolUsage: true,
			wantErr:   true,
			wantNil:   true,
		},
		{
			name:      "duplicate input type",
			input:     []string{IOTypeText, IOTypeText},
			output:    []string{IOTypeText},
			toolUsage: true,
			wantErr:   true,
			wantNil:   true,
		},
		{
			name:      "duplicate output type",
			input:     []string{IOTypeText},
			output:    []string{IOTypeText, IOTypeText},
			toolUsage: true,
			wantErr:   true,
			wantNil:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewCapabilities(tt.input, tt.output, tt.toolUsage)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewCapabilities() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if (got == nil) != tt.wantNil {
				t.Errorf("NewCapabilities() got = %v, wantNil %v", got, tt.wantNil)
			}
		})
	}
}

func boolPtr(b bool) *bool {
	return &b
}
