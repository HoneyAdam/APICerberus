package store

import (
	"testing"
)

// Test normalizeUserSortBy
func TestNormalizeUserSortBy(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"email", "email"},
		{"name", "name"},
		{"updated_at", "updated_at"},
		{"credit_balance", "credit_balance"},
		{"created_at", "created_at"},
		{"", "created_at"},
		{"unknown", "created_at"},
		{"EMAIL", "email"},
		{" Name ", "name"},
		{"Updated_AT", "updated_at"},
	}

	for _, tt := range tests {
		result := normalizeUserSortBy(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeUserSortBy(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

// Test validateUserInput
func TestValidateUserInput(t *testing.T) {
	tests := []struct {
		name    string
		user    User
		wantErr bool
	}{
		{
			name: "valid user",
			user: User{
				Email: "test@example.com",
				Name:  "Test User",
			},
			wantErr: false,
		},
		{
			name: "empty email",
			user: User{
				Email: "",
				Name:  "Test User",
			},
			wantErr: true,
		},
		{
			name: "whitespace email",
			user: User{
				Email: "   ",
				Name:  "Test User",
			},
			wantErr: true,
		},
		{
			name: "invalid email no at",
			user: User{
				Email: "testexample.com",
				Name:  "Test User",
			},
			wantErr: true,
		},
		{
			name: "invalid email no domain",
			user: User{
				Email: "test@",
				Name:  "Test User",
			},
			wantErr: true,
		},
		{
			name: "invalid email no dot",
			user: User{
				Email: "test@example",
				Name:  "Test User",
			},
			wantErr: true,
		},
		{
			name: "empty name",
			user: User{
				Email: "test@example.com",
				Name:  "",
			},
			wantErr: true,
		},
		{
			name: "whitespace name",
			user: User{
				Email: "test@example.com",
				Name:  "   ",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateUserInput(tt.user)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateUserInput() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Test looksLikeEmail
func TestLooksLikeEmail(t *testing.T) {
	tests := []struct {
		email  string
		valid  bool
	}{
		{"test@example.com", true},
		{"user@domain.org", true},
		{"a@b.co", true},
		{"testexample.com", false},
		{"test@", false},
		{"@example.com", false},
		{"test@example", false},
		{"", false},
		{"   ", false},
		{"test@@example.com", true},   // has @ and domain has .
		{"test@example.", true},       // has @ and domain has . (even at end)
		{".test@example.com", true},    // has @ and domain has .
	}

	for _, tt := range tests {
		result := looksLikeEmail(tt.email)
		if result != tt.valid {
			t.Errorf("looksLikeEmail(%q) = %v, want %v", tt.email, result, tt.valid)
		}
	}
}

// Test marshalJSON
func TestMarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		fallback string
		wantErr  bool
	}{
		{
			name:     "nil value",
			value:    nil,
			fallback: "null",
			wantErr:  false,
		},
		{
			name:     "string value",
			value:    "test",
			fallback: "",
			wantErr:  false,
		},
		{
			name:     "map value",
			value:    map[string]any{"key": "value"},
			fallback: "",
			wantErr:  false,
		},
		{
			name:     "slice value",
			value:    []string{"a", "b"},
			fallback: "",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := marshalJSON(tt.value, tt.fallback)
			if (err != nil) != tt.wantErr {
				t.Errorf("marshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.value == nil && result != tt.fallback {
				t.Errorf("marshalJSON() = %q, want %q", result, tt.fallback)
			}
		})
	}
}
