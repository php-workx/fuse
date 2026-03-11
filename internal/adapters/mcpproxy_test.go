package adapters

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/runger/fuse/internal/config"
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

func TestProxyAgentToDownstream_MalformedPayloadReturnsParseError(t *testing.T) {
	var downstream bytes.Buffer
	var agent bytes.Buffer

	input := rawMCPFrame([]byte(`{"jsonrpc":"2.0","id":1,"method":`))
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
		t.Fatalf("expected parse rejection to use id=null, got %#v", msg["id"])
	}
	errObj, _ := msg["error"].(map[string]interface{})
	if errObj == nil {
		t.Fatalf("expected JSON-RPC error response, got %#v", msg)
	}
	if code, _ := errObj["code"].(float64); code != -32700 {
		t.Fatalf("expected JSON-RPC parse error code -32700, got %#v", errObj["code"])
	}
	if downstream.Len() != 0 {
		t.Fatalf("expected malformed request not to be forwarded downstream, wrote %d bytes", downstream.Len())
	}
}

func TestRunMCPProxy_ReturnsWhenAgentClosesAndDownstreamStaysAlive(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("FUSE_HOME", tmpDir)

	configDir := filepath.Join(tmpDir, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	configYAML := "mcp_proxies:\n  - name: sleepy\n    command: /bin/sh\n    args:\n      - -c\n      - sleep 10\n    env: {}\n"
	if err := os.WriteFile(config.ConfigPath(), []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- RunMCPProxy("sleepy", strings.NewReader(""), io.Discard, io.Discard)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunMCPProxy returned error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("RunMCPProxy hung after agent EOF while downstream kept stdout open")
	}
}

func TestProxyAgentToDownstream_SensitiveResourceReadBlocked(t *testing.T) {
	var downstream bytes.Buffer
	var agent bytes.Buffer

	request := `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"~/.fuse/state/fuse.db"}}`
	input := rawMCPFrame([]byte(request))

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

	errObj, _ := msg["error"].(map[string]interface{})
	if errObj == nil {
		t.Fatalf("expected JSON-RPC error response, got %#v", msg)
	}
	message, _ := errObj["message"].(string)
	if !strings.Contains(message, "sensitive") {
		t.Fatalf("expected sensitive resource denial message, got %q", message)
	}
	if downstream.Len() != 0 {
		t.Fatalf("expected sensitive request not to be forwarded downstream, wrote %d bytes", downstream.Len())
	}
}

func TestProxyAgentToDownstream_NonSensitiveResourceForwarded(t *testing.T) {
	var downstream bytes.Buffer
	var agent bytes.Buffer

	request := `{"jsonrpc":"2.0","id":2,"method":"resources/read","params":{"uri":"/tmp/public.txt"}}`
	input := rawMCPFrame([]byte(request))

	err := proxyAgentToDownstream(strings.NewReader(input), &downstream, &agent, newInFlightRequests())
	if err != nil {
		t.Fatalf("proxyAgentToDownstream returned error: %v", err)
	}

	if downstream.Len() == 0 {
		t.Fatal("expected non-sensitive request to be forwarded downstream, but nothing was written")
	}

	payload, err := readMCPFrame(bufio.NewReader(&downstream))
	if err != nil {
		t.Fatalf("readMCPFrame(downstream): %v", err)
	}
	msg, err := decodeJSONRPC(payload)
	if err != nil {
		t.Fatalf("decodeJSONRPC(downstream): %v", err)
	}

	method, _ := msg["method"].(string)
	if method != "resources/read" {
		t.Fatalf("expected forwarded method resources/read, got %q", method)
	}
	params, _ := msg["params"].(map[string]interface{})
	uri, _ := params["uri"].(string)
	if uri != "/tmp/public.txt" {
		t.Fatalf("expected forwarded uri /tmp/public.txt, got %q", uri)
	}
	if agent.Len() != 0 {
		t.Fatalf("expected no error written to agent for safe resource, but %d bytes written", agent.Len())
	}
}

func rawMCPFrame(payload []byte) string {
	return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(payload), payload)
}
