package core

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/runger/fuse/internal/inspect"
)

// DefaultMaxBytes is the default maximum number of bytes to read from a file
// during inspection (1 MB).
const DefaultMaxBytes int64 = 1048576

// FileInspection holds the result of inspecting a referenced file.
type FileInspection struct {
	Path      string
	Exists    bool
	Size      int64
	Truncated bool
	Hash      string // SHA-256 hex
	Signals   []inspect.Signal
	Decision  Decision
	Reason    string
}

// InspectFile reads and inspects the file at path, scanning for dangerous
// patterns based on the file extension. If maxBytes <= 0, DefaultMaxBytes is
// used. Symlinks are resolved before reading.
func InspectFile(path string, maxBytes int64) (*FileInspection, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}

	// 1. Resolve symlinks.
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &FileInspection{
				Path:     path,
				Exists:   false,
				Decision: DecisionApproval,
				Reason:   "file not found",
			}, nil
		}
		return nil, err
	}

	// 2. Stat the file.
	info, err := os.Stat(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return &FileInspection{
				Path:     path,
				Exists:   false,
				Decision: DecisionApproval,
				Reason:   "file not found",
			}, nil
		}
		return nil, err
	}

	result := &FileInspection{
		Path:   resolved,
		Exists: true,
		Size:   info.Size(),
	}

	// 3. Check size and determine if truncation is needed.
	truncated := info.Size() > maxBytes
	result.Truncated = truncated

	// 4. Read file content (up to maxBytes).
	f, err := os.Open(resolved)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var content []byte
	hasher := sha256.New()
	if truncated {
		content = make([]byte, maxBytes)
		n, readErr := io.ReadFull(io.TeeReader(f, hasher), content)
		if readErr != nil && readErr != io.ErrUnexpectedEOF {
			return nil, readErr
		}
		content = content[:n]
		if _, err := io.Copy(hasher, f); err != nil {
			return nil, err
		}
	} else {
		content, err = io.ReadAll(io.TeeReader(f, hasher))
		if err != nil {
			return nil, err
		}
	}

	// 5. Compute SHA-256 hash of full file content.
	result.Hash = hex.EncodeToString(hasher.Sum(nil))

	// 6. Determine file type from extension and dispatch to scanner.
	ext := strings.ToLower(filepath.Ext(resolved))

	var signals []inspect.Signal

	switch ext {
	case ".py":
		signals = inspect.ScanPython(content)
	case ".sh", ".bash":
		signals = inspect.ScanShell(content)
	case ".js", ".ts", ".mjs", ".mts":
		signals = inspect.ScanJavaScript(content)
	case ".rb", ".pl", ".go":
		result.Decision = DecisionCaution
		result.Reason = "unsupported file type"
		return result, nil
	default:
		// Unknown extension, no scanner — treat as safe.
		result.Decision = DecisionSafe
		result.Reason = "no scanner for extension"
		return result, nil
	}

	result.Signals = signals

	// 7. Apply risk inference rules.

	// Truncated with no signals in the inspected portion: APPROVAL.
	if truncated && len(signals) == 0 {
		result.Decision = DecisionApproval
		result.Reason = "file truncated, partial scan found no signals"
		return result, nil
	}

	// No signals at all: SAFE.
	if len(signals) == 0 {
		result.Decision = DecisionSafe
		result.Reason = "no signals detected"
		return result, nil
	}

	// Signals present: use highest-severity signal category to decide.
	result.Decision = inferDecisionFromSignals(signals)
	result.Reason = "signals detected"

	return result, nil
}

// inferDecisionFromSignals returns the decision based on the highest-severity
// signal category found. Network/process signals yield APPROVAL; filesystem
// signals yield CAUTION; anything else yields CAUTION.
func inferDecisionFromSignals(signals []inspect.Signal) Decision {
	decision := DecisionCaution
	for _, s := range signals {
		switch s.Category {
		case "subprocess", "cloud_sdk", "cloud_cli", "http_control_plane",
			"dynamic_exec", "dynamic_import":
			// Network/process signals escalate to APPROVAL.
			return DecisionApproval
		case "destructive_fs", "destructive_verb":
			// Filesystem/destructive signals are CAUTION (keep looking
			// in case a higher severity category is also present).
			decision = DecisionCaution
		default:
			decision = DecisionCaution
		}
	}
	return decision
}

// DetectReferencedFile extracts the first positional argument that looks like
// a file path with a known extension from a sub-command string. It recognises
// invokers such as python, node, bash, ruby and perl.
//
// If scriptless-mode flags (-c, -e, -m) are present before a positional file
// argument, no file is returned.
func DetectReferencedFile(subCommand string) string {
	subCommand = strings.TrimSpace(subCommand)
	if subCommand == "" {
		return ""
	}

	parts := strings.Fields(subCommand)
	if len(parts) == 0 {
		return ""
	}

	invoker := filepath.Base(parts[0])
	args := parts[1:]

	switch invoker {
	case "python", "python3":
		return extractFile(args, []string{".py"}, []string{"-c", "-m"})
	case "node":
		return extractFile(args, []string{".js", ".ts"}, []string{"-e", "--eval", "-p", "--print"})
	case "bash", "sh":
		return extractFile(args, []string{".sh"}, []string{"-c"})
	case "ruby":
		return extractFile(args, []string{".rb"}, nil)
	case "perl":
		return extractFile(args, []string{".pl"}, []string{"-e"})
	default:
		return detectExecutablePath(parts[0])
	}
}

// extractFile walks the argument list looking for the first positional argument
// ending with one of the allowed extensions. If any of the scriptless flags is
// encountered before a positional file, it returns "".
func extractFile(args []string, exts []string, scriptlessFlags []string) string {
	scriptlessSet := make(map[string]bool, len(scriptlessFlags))
	for _, f := range scriptlessFlags {
		scriptlessSet[f] = true
	}

	for _, arg := range args {
		// If we encounter a scriptless flag, abort.
		if scriptlessSet[arg] {
			return ""
		}

		// Skip other flags (start with -).
		if strings.HasPrefix(arg, "-") {
			continue
		}

		// Positional argument: check if it has a recognised extension.
		lower := strings.ToLower(arg)
		for _, ext := range exts {
			if strings.HasSuffix(lower, ext) {
				return arg
			}
		}

		// First positional argument doesn't match any known extension.
		// Per the spec, we only look at the first positional argument.
		return ""
	}

	return ""
}

func detectExecutablePath(path string) string {
	if path == "" {
		return ""
	}
	if !strings.Contains(path, "/") {
		return ""
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return ""
	}
	if info.Mode()&0o111 == 0 {
		return ""
	}
	return path
}
