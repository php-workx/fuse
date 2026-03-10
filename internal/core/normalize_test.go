package core

import (
	"testing"
)

// ---------------------------------------------------------------------------
// DisplayNormalize tests
// ---------------------------------------------------------------------------

func TestDisplayNormalize_Whitespace(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "collapse multiple spaces",
			raw:  "terraform  destroy   PaymentsStack",
			want: "terraform destroy PaymentsStack",
		},
		{
			name: "trim leading and trailing spaces",
			raw:  "   ls -la   ",
			want: "ls -la",
		},
		{
			name: "collapse tabs to single space",
			raw:  "echo\t\thello",
			want: "echo hello",
		},
		{
			name: "mixed spaces and tabs",
			raw:  "  git  \t push \t origin  ",
			want: "git push origin",
		},
		{
			name: "single word no change",
			raw:  "ls",
			want: "ls",
		},
		{
			name: "empty string",
			raw:  "",
			want: "",
		},
		{
			name: "only whitespace",
			raw:  "   \t  ",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DisplayNormalize(tt.raw)
			if got != tt.want {
				t.Errorf("DisplayNormalize(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestDisplayNormalize_ANSI(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "strip color codes",
			raw:  "\x1b[31mhello\x1b[0m",
			want: "hello",
		},
		{
			name: "strip bold",
			raw:  "\x1b[1mwarning\x1b[0m text",
			want: "warning text",
		},
		{
			name: "strip multiple ANSI sequences",
			raw:  "\x1b[31m\x1b[1merror\x1b[0m: bad",
			want: "error: bad",
		},
		{
			name: "ANSI with cursor movement",
			raw:  "\x1b[2Jclear screen",
			want: "clear screen",
		},
		{
			name: "no ANSI untouched",
			raw:  "plain text here",
			want: "plain text here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DisplayNormalize(tt.raw)
			if got != tt.want {
				t.Errorf("DisplayNormalize(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestDisplayNormalize_Unicode(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "NFKC normalize fullwidth chars",
			// U+FF52 (fullwidth r) U+FF4D (fullwidth m) -> "rm"
			raw:  "\uff52\uff4d -rf /",
			want: "rm -rf /",
		},
		{
			name: "strip non-ASCII whitespace NBSP",
			raw:  "echo\u00a0hello",
			want: "echo hello",
		},
		{
			name: "strip em space",
			raw:  "echo\u2003hello",
			want: "echo hello",
		},
		{
			name: "strip zero-width space",
			raw:  "echo\u200bhello",
			want: "echohello",
		},
		{
			name: "strip control characters but keep newline",
			raw:  "line1\nline2",
			want: "line1\nline2",
		},
		{
			name: "strip control characters but keep tab",
			raw:  "col1\tcol2",
			want: "col1 col2",
		},
		{
			name: "strip BEL character",
			raw:  "hello\x07world",
			want: "helloworld",
		},
		{
			name: "strip soft hyphen (Cf)",
			raw:  "some\u00adcommand",
			want: "somecommand",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DisplayNormalize(tt.raw)
			if got != tt.want {
				t.Errorf("DisplayNormalize(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestDisplayNormalize_NullBytes(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "strip null bytes",
			raw:  "he\x00llo",
			want: "hello",
		},
		{
			name: "multiple null bytes",
			raw:  "\x00rm\x00 -rf\x00 /\x00",
			want: "rm -rf /",
		},
		{
			name: "only null bytes",
			raw:  "\x00\x00\x00",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DisplayNormalize(tt.raw)
			if got != tt.want {
				t.Errorf("DisplayNormalize(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ClassificationNormalize tests
// ---------------------------------------------------------------------------

func TestClassificationNormalize_Basename(t *testing.T) {
	tests := []struct {
		name string
		sub  string
		want string
	}{
		{
			name: "absolute path to rm",
			sub:  "/usr/bin/rm -rf /var/app",
			want: "rm -rf /var/app",
		},
		{
			name: "absolute path to terraform",
			sub:  "/usr/local/bin/terraform plan",
			want: "terraform plan",
		},
		{
			name: "no path unchanged",
			sub:  "git status",
			want: "git status",
		},
		{
			name: "relative path with slash",
			sub:  "./scripts/deploy.sh",
			want: "deploy.sh",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassificationNormalize(tt.sub)
			if got.Outer != tt.want {
				t.Errorf("ClassificationNormalize(%q).Outer = %q, want %q", tt.sub, got.Outer, tt.want)
			}
		})
	}
}

func TestClassificationNormalize_WrapperStripping(t *testing.T) {
	tests := []struct {
		name string
		sub  string
		want string
	}{
		{
			name: "sudo stripped",
			sub:  "sudo terraform destroy",
			want: "terraform destroy",
		},
		{
			name: "sudo -u deploy stripped",
			sub:  "sudo -u deploy rm -rf /var/app",
			want: "rm -rf /var/app",
		},
		{
			name: "nohup stripped",
			sub:  "nohup python script.py",
			want: "python script.py",
		},
		{
			name: "chained wrappers stripped",
			sub:  "sudo env nohup terraform destroy",
			want: "terraform destroy",
		},
		{
			name: "nice with priority stripped",
			sub:  "nice -n 10 python script.py",
			want: "python script.py",
		},
		{
			name: "nohup nice chained",
			sub:  "nohup nice -n 10 python script.py",
			want: "python script.py",
		},
		{
			name: "env with VAR=val stripped",
			sub:  "env FOO=bar baz=qux command_here",
			want: "command_here",
		},
		{
			name: "timeout with duration stripped",
			sub:  "timeout 30 mycommand arg1",
			want: "mycommand arg1",
		},
		{
			name: "strace stripped",
			sub:  "strace -o /tmp/trace.log ls -la",
			want: "ls -la",
		},
		{
			name: "ionice stripped",
			sub:  "ionice -c 2 -n 7 dd if=/dev/zero of=/dev/null",
			want: "dd if=/dev/zero of=/dev/null",
		},
		{
			name: "doas stripped",
			sub:  "doas rm -rf /tmp/junk",
			want: "rm -rf /tmp/junk",
		},
		{
			name: "env -i stripped",
			sub:  "env -i PATH=/usr/bin mycommand",
			want: "mycommand",
		},
		{
			name: "setsid stripped",
			sub:  "setsid mycommand arg",
			want: "mycommand arg",
		},
		{
			name: "chroot stripped",
			sub:  "chroot /mnt/root ls",
			want: "ls",
		},
		{
			name: "time stripped",
			sub:  "time make -j8",
			want: "make -j8",
		},
		{
			name: "command stripped",
			sub:  "command ls",
			want: "ls",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassificationNormalize(tt.sub)
			if got.Outer != tt.want {
				t.Errorf("ClassificationNormalize(%q).Outer = %q, want %q", tt.sub, got.Outer, tt.want)
			}
		})
	}
}

func TestClassificationNormalize_SudoEscalation(t *testing.T) {
	tests := []struct {
		name      string
		sub       string
		wantEscal bool
		wantOuter string
	}{
		{
			name:      "sudo sets escalation",
			sub:       "sudo terraform destroy",
			wantEscal: true,
			wantOuter: "terraform destroy",
		},
		{
			name:      "doas sets escalation",
			sub:       "doas rm -rf /",
			wantEscal: true,
			wantOuter: "rm -rf /",
		},
		{
			name:      "no wrapper no escalation",
			sub:       "terraform destroy",
			wantEscal: false,
			wantOuter: "terraform destroy",
		},
		{
			name:      "nohup does not set escalation",
			sub:       "nohup python script.py",
			wantEscal: false,
			wantOuter: "python script.py",
		},
		{
			name:      "sudo in chain sets escalation",
			sub:       "sudo env nohup terraform destroy",
			wantEscal: true,
			wantOuter: "terraform destroy",
		},
		{
			name:      "sudo -u deploy sets escalation",
			sub:       "sudo -u deploy rm -rf /var/app",
			wantEscal: true,
			wantOuter: "rm -rf /var/app",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassificationNormalize(tt.sub)
			if got.EscalateClassification != tt.wantEscal {
				t.Errorf("ClassificationNormalize(%q).EscalateClassification = %v, want %v",
					tt.sub, got.EscalateClassification, tt.wantEscal)
			}
			if got.Outer != tt.wantOuter {
				t.Errorf("ClassificationNormalize(%q).Outer = %q, want %q",
					tt.sub, got.Outer, tt.wantOuter)
			}
		})
	}
}

func TestClassificationNormalize_BashCExtraction(t *testing.T) {
	tests := []struct {
		name      string
		sub       string
		wantOuter string
		wantInner []string
	}{
		{
			name:      "bash -c simple",
			sub:       `bash -c "terraform destroy"`,
			wantOuter: `bash -c terraform destroy`,
			wantInner: []string{"terraform destroy"},
		},
		{
			name:      "sh -c simple",
			sub:       `sh -c "rm -rf /"`,
			wantOuter: `sh -c rm -rf /`,
			wantInner: []string{"rm -rf /"},
		},
		{
			name:      "bash -c with single quotes",
			sub:       `bash -c 'terraform destroy'`,
			wantOuter: `bash -c terraform destroy`,
			wantInner: []string{"terraform destroy"},
		},
		{
			name:      "bash -lc combined flag",
			sub:       `bash -lc "terraform destroy"`,
			wantOuter: `bash -lc terraform destroy`,
			wantInner: []string{"terraform destroy"},
		},
		{
			name:      "sh -xc combined flag",
			sub:       `sh -xc "echo hello"`,
			wantOuter: `sh -xc echo hello`,
			wantInner: []string{"echo hello"},
		},
		{
			name:      "bash -l -c separate flags",
			sub:       `bash -l -c "terraform destroy"`,
			wantOuter: `bash -l -c terraform destroy`,
			wantInner: []string{"terraform destroy"},
		},
		{
			name:      "no -c flag not extracted",
			sub:       "bash script.sh",
			wantOuter: "bash script.sh",
			wantInner: nil,
		},
		{
			name:      "sudo bash -c extraction with escalation",
			sub:       `sudo bash -c "terraform destroy"`,
			wantOuter: `bash -c terraform destroy`,
			wantInner: []string{"terraform destroy"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassificationNormalize(tt.sub)
			if got.Outer != tt.wantOuter {
				t.Errorf("ClassificationNormalize(%q).Outer = %q, want %q",
					tt.sub, got.Outer, tt.wantOuter)
			}
			if !stringSliceEqual(got.Inner, tt.wantInner) {
				t.Errorf("ClassificationNormalize(%q).Inner = %v, want %v",
					tt.sub, got.Inner, tt.wantInner)
			}
		})
	}
}

func TestClassificationNormalize_SshExtraction(t *testing.T) {
	tests := []struct {
		name      string
		sub       string
		wantOuter string
		wantInner []string
	}{
		{
			name:      "ssh simple",
			sub:       `ssh prod 'terraform destroy'`,
			wantOuter: `ssh prod terraform destroy`,
			wantInner: []string{"terraform destroy"},
		},
		{
			name:      "ssh user@host",
			sub:       "ssh user@prod terraform plan",
			wantOuter: "ssh user@prod terraform plan",
			wantInner: []string{"terraform plan"},
		},
		{
			name:      "ssh with -i flag",
			sub:       "ssh -i /path/to/key prod terraform destroy",
			wantOuter: "ssh -i /path/to/key prod terraform destroy",
			wantInner: []string{"terraform destroy"},
		},
		{
			name:      "ssh with port flag",
			sub:       "ssh -p 2222 prod ls -la",
			wantOuter: "ssh -p 2222 prod ls -la",
			wantInner: []string{"ls -la"},
		},
		{
			name:      "ssh with multiple flags",
			sub:       "ssh -t -o StrictHostKeyChecking=no prod rm -rf /tmp/junk",
			wantOuter: "ssh -t -o StrictHostKeyChecking=no prod rm -rf /tmp/junk",
			wantInner: []string{"rm -rf /tmp/junk"},
		},
		{
			name:      "ssh no remote command",
			sub:       "ssh prod",
			wantOuter: "ssh prod",
			wantInner: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassificationNormalize(tt.sub)
			if got.Outer != tt.wantOuter {
				t.Errorf("ClassificationNormalize(%q).Outer = %q, want %q",
					tt.sub, got.Outer, tt.wantOuter)
			}
			if !stringSliceEqual(got.Inner, tt.wantInner) {
				t.Errorf("ClassificationNormalize(%q).Inner = %v, want %v",
					tt.sub, got.Inner, tt.wantInner)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Spec table integration tests
// ---------------------------------------------------------------------------

func TestDisplayNormalize_SpecExamples(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "whitespace collapsed",
			raw:  "terraform  destroy   PaymentsStack",
			want: "terraform destroy PaymentsStack",
		},
		{
			name: "sudo preserved in display",
			raw:  "sudo terraform destroy",
			want: "sudo terraform destroy",
		},
		{
			name: "sudo -u deploy preserved in display",
			raw:  "sudo -u deploy rm -rf /var/app",
			want: "sudo -u deploy rm -rf /var/app",
		},
		{
			name: "absolute path preserved in display",
			raw:  "/usr/bin/rm -rf /var/app",
			want: "/usr/bin/rm -rf /var/app",
		},
		{
			name: "nohup nice preserved in display",
			raw:  "nohup nice -n 10 python script.py",
			want: "nohup nice -n 10 python script.py",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DisplayNormalize(tt.raw)
			if got != tt.want {
				t.Errorf("DisplayNormalize(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestClassificationNormalize_SpecExamples(t *testing.T) {
	tests := []struct {
		name      string
		sub       string
		wantOuter string
		wantInner []string
		wantEscal bool
	}{
		{
			name:      "whitespace collapsed",
			sub:       "terraform destroy PaymentsStack",
			wantOuter: "terraform destroy PaymentsStack",
		},
		{
			name:      "sudo stripped with escalation",
			sub:       "sudo terraform destroy",
			wantOuter: "terraform destroy",
			wantEscal: true,
		},
		{
			name:      "sudo -u deploy stripped",
			sub:       "sudo -u deploy rm -rf /var/app",
			wantOuter: "rm -rf /var/app",
			wantEscal: true,
		},
		{
			name:      "basename extracted",
			sub:       "/usr/bin/rm -rf /var/app",
			wantOuter: "rm -rf /var/app",
		},
		{
			name:      "bash -c inner extracted",
			sub:       `bash -c "terraform destroy"`,
			wantOuter: `bash -c terraform destroy`,
			wantInner: []string{"terraform destroy"},
		},
		{
			name:      "bash -c rm inner extracted",
			sub:       `bash -c 'rm -rf /'`,
			wantOuter: `bash -c rm -rf /`,
			wantInner: []string{"rm -rf /"},
		},
		{
			name:      "ssh remote cmd extracted",
			sub:       `ssh prod 'terraform destroy'`,
			wantOuter: `ssh prod terraform destroy`,
			wantInner: []string{"terraform destroy"},
		},
		{
			name:      "nohup nice wrappers stripped",
			sub:       "nohup nice -n 10 python script.py",
			wantOuter: "python script.py",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassificationNormalize(tt.sub)
			if got.Outer != tt.wantOuter {
				t.Errorf("ClassificationNormalize(%q).Outer = %q, want %q",
					tt.sub, got.Outer, tt.wantOuter)
			}
			if !stringSliceEqual(got.Inner, tt.wantInner) {
				t.Errorf("ClassificationNormalize(%q).Inner = %v, want %v",
					tt.sub, got.Inner, tt.wantInner)
			}
			if got.EscalateClassification != tt.wantEscal {
				t.Errorf("ClassificationNormalize(%q).EscalateClassification = %v, want %v",
					tt.sub, got.EscalateClassification, tt.wantEscal)
			}
		})
	}
}

func TestClassificationNormalize_BashCExtractionFailure(t *testing.T) {
	tests := []struct {
		name               string
		sub                string
		wantExtrFailed     bool
		wantInnerEmpty     bool
	}{
		{
			name:           "unbalanced single quote",
			sub:            "bash -c 'echo hello",
			wantExtrFailed: true,
			wantInnerEmpty: true,
		},
		{
			name:           "unbalanced double quote",
			sub:            `bash -c "echo hello`,
			wantExtrFailed: true,
			wantInnerEmpty: true,
		},
		{
			name:           "sh -c unbalanced quote",
			sub:            "sh -c 'echo hello",
			wantExtrFailed: true,
			wantInnerEmpty: true,
		},
		{
			name:           "bash -c with valid quotes does not fail",
			sub:            "bash -c 'echo hello'",
			wantExtrFailed: false,
			wantInnerEmpty: false,
		},
		{
			name:           "bash without -c does not set extraction failed",
			sub:            "bash script.sh",
			wantExtrFailed: false,
			wantInnerEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassificationNormalize(tt.sub)
			if got.ExtractionFailed != tt.wantExtrFailed {
				t.Errorf("ClassificationNormalize(%q).ExtractionFailed = %v, want %v",
					tt.sub, got.ExtractionFailed, tt.wantExtrFailed)
			}
			if tt.wantInnerEmpty && len(got.Inner) != 0 {
				t.Errorf("ClassificationNormalize(%q).Inner = %v, want empty",
					tt.sub, got.Inner)
			}
			if !tt.wantInnerEmpty && len(got.Inner) == 0 {
				t.Errorf("ClassificationNormalize(%q).Inner is empty, want non-empty",
					tt.sub)
			}
		})
	}
}

// stringSliceEqual compares two string slices for equality, treating nil and empty as equal.
func stringSliceEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
