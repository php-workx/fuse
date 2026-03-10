package adapters

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
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

func TestProxyAgentToDownstream_OversizeHeaderWithoutBodyDoesNotHang(t *testing.T) {
	pr, pw := io.Pipe()
	var downstream bytes.Buffer
	var agent bytes.Buffer

	done := make(chan error, 1)
	go func() {
		done <- proxyAgentToDownstream(pr, &downstream, &agent, newInFlightRequests())
	}()

	if _, err := fmt.Fprintf(pw, "Content-Length: %d\r\n\r\n", maxMCPFrameBytes+1024); err != nil {
		t.Fatalf("write header: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("proxyAgentToDownstream returned error: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("proxyAgentToDownstream hung while rejecting oversized header")
	}
}

func TestProxyDownstreamToAgent_ForwardsMalformedJSONPayload(t *testing.T) {
	var agent bytes.Buffer
	requests := newInFlightRequests()
	requests.add(float64(1), "tools/call")

	input := rawMCPFrame([]byte(`{"jsonrpc":"2.0","id":1,"result":`))
	err := proxyDownstreamToAgent(strings.NewReader(input), &agent, requests)
	if err != nil {
		t.Fatalf("proxyDownstreamToAgent returned error: %v", err)
	}

	payload, err := readMCPFrame(bufio.NewReader(&agent))
	if err != nil {
		t.Fatalf("readMCPFrame(agent): %v", err)
	}
	if string(payload) != `{"jsonrpc":"2.0","id":1,"result":` {
		t.Fatalf("expected malformed payload forwarded unchanged, got %q", string(payload))
	}
}

func rawMCPFrame(payload []byte) string {
	return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(payload), payload)
}
