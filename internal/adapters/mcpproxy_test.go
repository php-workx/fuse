package adapters

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestProxyAgentToDownstream_OversizeRequestReturnsJSONRPCError(t *testing.T) {
	var downstream bytes.Buffer
	var agent bytes.Buffer
	input := oversizedMCPFrame(maxMCPFrameBytes + 1)

	err := proxyAgentToDownstream(strings.NewReader(input), &downstream, &agent, newInFlightRequests())
	if err != nil {
		t.Fatalf("proxyAgentToDownstream returned error: %v", err)
	}

	payload, err := readMCPFrame(bufio.NewReader(&agent))
	if err != nil {
		t.Fatalf("readMCPFrame(agent): %v", err)
	}
	msg, err := decodeJSONRPC(payload)
	if err != nil {
		t.Fatalf("decodeJSONRPC(agent): %v", err)
	}

	if msg["id"] != nil {
		t.Fatalf("expected oversize rejection to use id=null, got %#v", msg["id"])
	}
	errObj, _ := msg["error"].(map[string]interface{})
	if errObj == nil {
		t.Fatalf("expected JSON-RPC error response, got %#v", msg)
	}
	message, _ := errObj["message"].(string)
	if !strings.Contains(message, "exceeds limit") {
		t.Fatalf("expected oversize error message, got %#v", errObj)
	}
	if downstream.Len() != 0 {
		t.Fatalf("expected oversized request not to be forwarded downstream, wrote %d bytes", downstream.Len())
	}
}

func oversizedMCPFrame(contentLength int) string {
	return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", contentLength, strings.Repeat("x", contentLength))
}
