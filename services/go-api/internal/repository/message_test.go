package repository

import (
	"errors"
	"testing"

	drivermysql "github.com/go-sql-driver/mysql"
)

func TestIsDuplicateClientMessageID(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		hasID   bool
		wantDup bool
	}{
		{
			name:    "named client-message unique index",
			err:     &drivermysql.MySQLError{Number: 1062, Message: "Duplicate entry 's-m' for key 'uq_messages_session_client'"},
			hasID:   true,
			wantDup: true,
		},
		{
			name:    "wrapped driver error",
			err:     errors.Join(errors.New("transaction failed"), &drivermysql.MySQLError{Number: 1062, Message: "Duplicate entry for key 'uq_messages_session_client'"}),
			hasID:   true,
			wantDup: true,
		},
		{
			name:    "different unique index",
			err:     &drivermysql.MySQLError{Number: 1062, Message: "Duplicate entry for key 'uq_messages_session_seq'"},
			hasID:   true,
			wantDup: false,
		},
		{
			name:    "assistant message has no client ID",
			err:     &drivermysql.MySQLError{Number: 1062, Message: "Duplicate entry for key 'uq_messages_session_client'"},
			hasID:   false,
			wantDup: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDuplicateClientMessageID(tt.err, tt.hasID); got != tt.wantDup {
				t.Fatalf("isDuplicateClientMessageID() = %v, want %v", got, tt.wantDup)
			}
		})
	}
}
