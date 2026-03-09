package inspect

import (
	"testing"
)

func TestScanShell_SafeScript(t *testing.T) {
	content := readTestFile(t, "safe_script.sh")
	signals := ScanShell(content)

	if len(signals) != 0 {
		t.Errorf("expected 0 signals for safe_script.sh, got %d:", len(signals))
		for _, s := range signals {
			t.Logf("  line %d: category=%s match=%q", s.Line, s.Category, s.Match)
		}
	}
}

func TestScanShell_DangerousScript(t *testing.T) {
	content := readTestFile(t, "dangerous_script.sh")
	signals := ScanShell(content)

	if len(signals) == 0 {
		t.Fatal("expected signals for dangerous_script.sh, got 0")
	}

	categories := signalCategories(signals)

	expectedCategories := []string{"destructive_fs", "cloud_cli", "destructive_verb", "subprocess", "http_control_plane"}
	for _, cat := range expectedCategories {
		if !categories[cat] {
			t.Errorf("expected category %q in signals, not found", cat)
		}
	}

	t.Logf("found %d signals:", len(signals))
	for _, s := range signals {
		t.Logf("  line %d: category=%s match=%q", s.Line, s.Category, s.Match)
	}
}

func TestScanShell_CommentSkipping(t *testing.T) {
	content := []byte(`#!/bin/bash
# rm -rf /
# eval "malicious"
# aws s3 rm s3://bucket
echo "safe"
`)
	signals := ScanShell(content)

	if len(signals) != 0 {
		t.Errorf("expected 0 signals for commented-out code, got %d:", len(signals))
		for _, s := range signals {
			t.Logf("  line %d: category=%s match=%q", s.Line, s.Category, s.Match)
		}
	}
}

func TestScanShell_SpecificPatterns(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		category string
	}{
		{"rm -rf", `rm -rf /tmp/data`, "destructive_fs"},
		{"rm -fr", `rm -fr /tmp/data`, "destructive_fs"},
		{"mkfs", `mkfs -t ext4 /dev/sdb`, "destructive_fs"},
		{"dd of=", `dd if=/dev/zero of=/dev/sda bs=1M`, "destructive_fs"},
		{"aws cli", `aws s3 ls`, "cloud_cli"},
		{"gcloud", `gcloud compute instances list`, "cloud_cli"},
		{"az cli", `az vm list`, "cloud_cli"},
		{"oci cli", `oci compute instance list`, "cloud_cli"},
		{"curl DELETE", `curl -X DELETE https://api.example.com/resource`, "http_control_plane"},
		{"kubectl delete", `kubectl delete pod my-pod`, "destructive_verb"},
		{"terraform destroy", `terraform destroy -auto-approve`, "destructive_verb"},
		{"terraform apply", `terraform apply -auto-approve`, "destructive_verb"},
		{"eval", `eval "$CMD"`, "subprocess"},
		{"command substitution", `RESULT=$(whoami)`, "subprocess"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signals := ScanShell([]byte(tt.code))
			found := false
			for _, s := range signals {
				if s.Category == tt.category {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected category %q for code %q, got signals: %v", tt.category, tt.code, signals)
			}
		})
	}
}
