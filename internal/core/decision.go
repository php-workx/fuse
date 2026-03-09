package core

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
)

// Decision represents the safety classification of a command.
type Decision string

const (
	DecisionSafe     Decision = "SAFE"
	DecisionCaution  Decision = "CAUTION"
	DecisionApproval Decision = "APPROVAL"
	DecisionBlocked  Decision = "BLOCKED"
)

// decisionSeverity maps decisions to their severity level for comparison.
var decisionSeverity = map[Decision]int{
	DecisionSafe:     0,
	DecisionCaution:  1,
	DecisionApproval: 2,
	DecisionBlocked:  3,
}

// MaxDecision returns the most restrictive of two decisions.
func MaxDecision(a, b Decision) Decision {
	if decisionSeverity[a] >= decisionSeverity[b] {
		return a
	}
	return b
}

// ShellRequest represents an incoming command to classify.
type ShellRequest struct {
	RawCommand string
	Cwd        string
	Source     string
	SessionID  string
}

// ClassifiedCommand holds the result of classification normalization.
type ClassifiedCommand struct {
	Outer                  string
	Inner                  []string
	EscalateClassification bool
}

// ComputeDecisionKey produces a SHA-256 hash used for approval record lookup.
// Uses length-prefixed fields: source + displayNormalized + fileHash.
func ComputeDecisionKey(source, displayNormalized, fileHash string) string {
	h := sha256.New()
	writeField(h, source)
	writeField(h, displayNormalized)
	writeField(h, fileHash)
	return hex.EncodeToString(h.Sum(nil))
}

// writeField writes a length-prefixed string to the hash.
func writeField(h hash.Hash, s string) {
	length := make([]byte, 4)
	binary.BigEndian.PutUint32(length, uint32(len(s)))
	h.Write(length)
	h.Write([]byte(s))
}
