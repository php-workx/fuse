package adapters

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const maxMCPFrameBytes = 1 << 20

type jsonRPCMessage map[string]interface{}

type mcpFrameTooLargeError struct {
	contentLength int
}

func (e *mcpFrameTooLargeError) Error() string {
	return fmt.Sprintf("content length %d exceeds limit %d", e.contentLength, maxMCPFrameBytes)
}

func readMCPFrame(r *bufio.Reader) ([]byte, error) {
	contentLength := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}

		name, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("invalid MCP header line %q", line)
		}
		if strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return nil, fmt.Errorf("invalid content length: %w", err)
			}
			contentLength = n
		}
	}

	if contentLength < 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}
	if contentLength > maxMCPFrameBytes {
		return nil, &mcpFrameTooLargeError{contentLength: contentLength}
	}

	payload := make([]byte, contentLength)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func writeMCPFrame(w io.Writer, payload []byte) error {
	if len(payload) > maxMCPFrameBytes {
		return fmt.Errorf("content length %d exceeds limit %d", len(payload), maxMCPFrameBytes)
	}
	if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(payload)); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

func decodeJSONRPC(payload []byte) (jsonRPCMessage, error) {
	var msg jsonRPCMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		return nil, err
	}
	return msg, nil
}

func encodeJSONRPC(msg jsonRPCMessage) ([]byte, error) {
	return json.Marshal(msg)
}

func jsonRPCIDKey(id interface{}) string {
	if id == nil {
		return ""
	}
	data, err := json.Marshal(id)
	if err != nil {
		return fmt.Sprintf("%v", id)
	}
	return string(data)
}
