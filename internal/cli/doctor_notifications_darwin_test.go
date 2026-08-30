//go:build darwin

package cli

import (
	"errors"
	"strings"
	"testing"
)

func TestAssessNotifierSignatureRequiresConfiguredReleaseIdentityAndGatekeeper(t *testing.T) {
	tests := []struct {
		name       string
		identity   string
		configured string
		gatekeeper error
		want       string
		wantErr    string
	}{
		{name: "development ad hoc", identity: "Signature=adhoc\nTeamIdentifier=not set", want: "ad-hoc signature"},
		{name: "release ad hoc", identity: "Signature=adhoc\nTeamIdentifier=not set", configured: "TEAM123", wantErr: "ad-hoc"},
		{name: "wrong team", identity: "Signature=Developer ID Application\nTeamIdentifier=OTHER", configured: "TEAM123", wantErr: "does not match"},
		{name: "gatekeeper rejected", identity: "Signature=Developer ID Application\nTeamIdentifier=TEAM123", configured: "TEAM123", gatekeeper: errors.New("rejected"), wantErr: "Gatekeeper"},
		{name: "healthy release", identity: "Signature=Developer ID Application\nTeamIdentifier=TEAM123", configured: "TEAM123", want: "Developer ID Team ID TEAM123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := assessNotifierSignature(tt.identity, tt.configured, func() error { return tt.gatekeeper })
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("assessNotifierSignature() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("assessNotifierSignature() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("assessNotifierSignature() = %q, want %q", got, tt.want)
			}
		})
	}
}
