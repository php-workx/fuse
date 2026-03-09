package cli

import "testing"

func TestParseRunCommandArg(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{
			name: "single command string",
			args: []string{"git status"},
			want: "git status",
		},
		{
			name:    "no command",
			args:    nil,
			wantErr: true,
		},
		{
			name:    "multiple command args rejected",
			args:    []string{"git", "status"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSingleCommandArg(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseSingleCommandArg(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}
