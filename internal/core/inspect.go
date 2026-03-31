package core

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/php-workx/fuse/internal/inspect"
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
	if !info.Mode().IsRegular() {
		result.Decision = DecisionApproval
		result.Reason = "non-regular file requires approval"
		return result, nil
	}

	// 3. Check size and determine if truncation is needed.
	truncated := info.Size() > maxBytes
	result.Truncated = truncated

	// 4. Read file content (up to maxBytes).
	content, hash, err := readFileForInspection(resolved, maxBytes, truncated)
	if err != nil {
		return nil, err
	}
	result.Hash = hash

	// 5. Dispatch to scanner and infer decision.
	return dispatchScannerAndInfer(result, resolved, content, truncated)
}

// readFileForInspection reads the file content (up to maxBytes) and computes its SHA-256 hash.
func readFileForInspection(resolved string, maxBytes int64, truncated bool) ([]byte, string, error) {
	f, err := os.Open(resolved)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = f.Close() }()

	var content []byte
	hasher := sha256.New()
	if truncated {
		content = make([]byte, maxBytes)
		n, readErr := io.ReadFull(io.TeeReader(f, hasher), content)
		if readErr != nil && readErr != io.ErrUnexpectedEOF {
			return nil, "", readErr
		}
		content = content[:n]
		if _, copyErr := io.Copy(hasher, f); copyErr != nil {
			return nil, "", copyErr
		}
	} else {
		content, err = io.ReadAll(io.TeeReader(f, hasher))
		if err != nil {
			return nil, "", err
		}
	}
	return content, hex.EncodeToString(hasher.Sum(nil)), nil
}

// dispatchScannerAndInfer selects the scanner based on file extension, runs it,
// and infers the decision from any detected signals.
func dispatchScannerAndInfer(result *FileInspection, resolved string, content []byte, truncated bool) (*FileInspection, error) {
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
		result.Decision = DecisionCaution
		result.Reason = "unknown file type, no scanner available"
		return result, nil
	}

	result.Signals = signals
	inferSignalDecision(result, signals, truncated)
	return result, nil
}

// inferSignalDecision sets the decision and reason on the result based on detected signals.
func inferSignalDecision(result *FileInspection, signals []inspect.Signal, truncated bool) {
	if truncated && len(signals) == 0 {
		result.Decision = DecisionApproval
		result.Reason = "file truncated, partial scan found no signals"
		return
	}
	if len(signals) == 0 {
		result.Decision = DecisionSafe
		result.Reason = "no signals detected"
		return
	}
	result.Decision = inferDecisionFromSignals(signals)
	result.Reason = "signals detected"
}

// inferDecisionFromSignals returns the decision based on the highest-severity
// signal category found. Network/process signals yield APPROVAL; cloud_sdk
// alone yields CAUTION but combined with destructive or subprocess signals
// yields APPROVAL; anything else yields CAUTION.
func inferDecisionFromSignals(signals []inspect.Signal) Decision {
	decision := DecisionCaution
	hasCloudSDK := false
	hasDestructive := false
	for _, s := range signals {
		switch s.Category {
		case "subprocess", "cloud_cli", "http_control_plane",
			"dynamic_exec", "dynamic_import":
			return DecisionCaution
		case "cloud_sdk":
			hasCloudSDK = true
		case "destructive_fs", "destructive_verb":
			hasDestructive = true
		default:
			decision = DecisionCaution
		}
	}
	if hasCloudSDK && hasDestructive {
		return DecisionCaution
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

	parts, unbalancedQuotes := tokenizeQuoteAware(subCommand)
	if unbalancedQuotes || len(parts) == 0 {
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
func extractFile(args, exts, scriptlessFlags []string) string {
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
