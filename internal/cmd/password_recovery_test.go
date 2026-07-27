package cmd

import (
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsAuthenticationError(t *testing.T) {
	testCases := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name: "PostgreSQL error code 28P01 (invalid_password)",
			err: &pgconn.PgError{
				Code:    "28P01",
				Message: "password authentication failed for user \"test\"",
			},
			expected: true,
		},
		{
			name: "PostgreSQL error code 28000 (invalid_authorization_specification)",
			err: &pgconn.PgError{
				Code:    "28000",
				Message: "role \"nonexistent\" does not exist",
			},
			expected: true,
		},
		{
			name: "PostgreSQL error code 57P03 (cannot_connect_now) - not auth error",
			err: &pgconn.PgError{
				Code:    "57P03",
				Message: "the database system is starting up",
			},
			expected: false,
		},
		{
			name: "PostgreSQL error code 3D000 (database does not exist) - not auth error",
			err: &pgconn.PgError{
				Code:    "3D000",
				Message: "database \"nonexistent\" does not exist",
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isAuthenticationError(tc.err)

			if result != tc.expected {
				t.Errorf("Expected isAuthenticationError to return %v for error %v, got %v",
					tc.expected, tc.err, result)
			}
		})
	}
}
