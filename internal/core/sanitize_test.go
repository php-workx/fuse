package core

import "testing"

func TestSanitize_SingleQuotes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "dangerous content in single quotes",
			input: "echo 'rm -rf /'",
			want:  "echo __SQ__",
		},
		{
			name:  "multiple single-quoted strings",
			input: "grep 'pattern1' file | grep 'pattern2'",
			want:  "grep __SQ__ file | grep __SQ__",
		},
		{
			name:  "single-quoted git command",
			input: "grep 'git reset --hard' file.md",
			want:  "grep __SQ__ file.md",
		},
		{
			name:  "empty single-quoted string",
			input: "echo ''",
			want:  "echo __SQ__",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeForClassification(tt.input, false)
			if got != tt.want {
				t.Errorf("SanitizeForClassification(%q, false) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitize_DoubleQuotesPreserved(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "double quotes preserved when not safe verb",
			input: `bash -c "terraform destroy"`,
			want:  `bash -c "terraform destroy"`,
		},
		{
			name:  "double quotes with dangerous content preserved",
			input: `bash -c "rm -rf /"`,
			want:  `bash -c "rm -rf /"`,
		},
		{
			name:  "mixed quotes without safe verb",
			input: `bash -c "echo 'hello'"`,
			want:  `bash -c "echo __SQ__"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeForClassification(tt.input, false)
			if got != tt.want {
				t.Errorf("SanitizeForClassification(%q, false) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitize_KnownSafeVerb(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "echo with double-quoted dangerous content",
			input: `echo "terraform destroy"`,
			want:  `echo __DQ__`,
		},
		{
			name:  "grep with double-quoted pattern",
			input: `grep "terraform destroy" logfile.txt`,
			want:  `grep __DQ__ logfile.txt`,
		},
		{
			name:  "printf with double-quoted content",
			input: `printf "rm -rf /"`,
			want:  `printf __DQ__`,
		},
		{
			name:  "safe verb with both quote types",
			input: `echo 'single' "double"`,
			want:  `echo __SQ__ __DQ__`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeForClassification(tt.input, true)
			if got != tt.want {
				t.Errorf("SanitizeForClassification(%q, true) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitize_Comments(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		knownSafeVerb bool
		want          string
	}{
		{
			name:          "trailing comment stripped",
			input:         "git push origin main # deploy to prod",
			knownSafeVerb: false,
			want:          "git push origin main",
		},
		{
			name:          "comment with multiple spaces before hash",
			input:         "ls -la   # list all files",
			knownSafeVerb: false,
			want:          "ls -la",
		},
		{
			name:          "no comment present",
			input:         "git status",
			knownSafeVerb: false,
			want:          "git status",
		},
		{
			name:          "hash inside single quotes not stripped",
			input:         "echo '# not a comment' # real comment",
			knownSafeVerb: false,
			want:          "echo __SQ__",
		},
		{
			name:          "hash without leading space is not a comment",
			input:         "echo foo#bar",
			knownSafeVerb: false,
			want:          "echo foo#bar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeForClassification(tt.input, tt.knownSafeVerb)
			if got != tt.want {
				t.Errorf("SanitizeForClassification(%q, %v) = %q, want %q", tt.input, tt.knownSafeVerb, got, tt.want)
			}
		})
	}
}

func TestSanitize_NestedQuotes(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		knownSafeVerb bool
		want          string
	}{
		{
			name:          "double quotes inside single quotes",
			input:         `echo '"hello"'`,
			knownSafeVerb: false,
			want:          "echo __SQ__",
		},
		{
			name:          "single quotes inside double quotes when not safe verb",
			input:         `bash -c "echo 'hello'"`,
			knownSafeVerb: false,
			want:          `bash -c "echo __SQ__"`,
		},
		{
			name:          "single quotes inside double quotes when safe verb",
			input:         `echo "it's fine"`,
			knownSafeVerb: true,
			want:          `echo __DQ__`,
		},
		{
			name:          "adjacent single-quoted strings",
			input:         "echo 'a''b'",
			knownSafeVerb: false,
			want:          "echo __SQ____SQ__",
		},
		{
			name:          "mixed nesting with safe verb",
			input:         `grep 'pattern' file | echo "result: 'ok'"`,
			knownSafeVerb: true,
			want:          "grep __SQ__ file | echo __DQ__",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeForClassification(tt.input, tt.knownSafeVerb)
			if got != tt.want {
				t.Errorf("SanitizeForClassification(%q, %v) = %q, want %q", tt.input, tt.knownSafeVerb, got, tt.want)
			}
		})
	}
}

func TestSanitize_EmptyInput(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		knownSafeVerb bool
		want          string
	}{
		{
			name:          "empty string not safe verb",
			input:         "",
			knownSafeVerb: false,
			want:          "",
		},
		{
			name:          "empty string safe verb",
			input:         "",
			knownSafeVerb: true,
			want:          "",
		},
		{
			name:          "whitespace only",
			input:         "   ",
			knownSafeVerb: false,
			want:          "   ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeForClassification(tt.input, tt.knownSafeVerb)
			if got != tt.want {
				t.Errorf("SanitizeForClassification(%q, %v) = %q, want %q", tt.input, tt.knownSafeVerb, got, tt.want)
			}
		})
	}
}

func TestKnownSafeVerbs(t *testing.T) {
	expected := []string{"echo", "printf", "grep", "awk", "sed", "cat", "log"}
	for _, verb := range expected {
		if !KnownSafeVerbs[verb] {
			t.Errorf("KnownSafeVerbs[%q] = false, want true", verb)
		}
	}

	unexpected := []string{"bash", "rm", "terraform", "ssh", ""}
	for _, verb := range unexpected {
		if KnownSafeVerbs[verb] {
			t.Errorf("KnownSafeVerbs[%q] = true, want false", verb)
		}
	}
}
