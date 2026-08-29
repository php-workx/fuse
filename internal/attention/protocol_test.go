package attention

import (
	"bufio"
	"bytes"
	"testing"
)

func TestReadProtocolLineRejectsOversizedInput(t *testing.T) {
	reader := bufio.NewReader(bytes.NewReader(append(bytes.Repeat([]byte{'x'}, maximumProtocolLineBytes), '\n')))
	if _, err := readProtocolLine(reader); err == nil {
		t.Fatal("oversized protocol line was accepted")
	}
}
