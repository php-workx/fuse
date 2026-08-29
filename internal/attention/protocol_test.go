package attention

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"sync"
	"testing"
	"time"
)

func TestReadProtocolLineRejectsOversizedInput(t *testing.T) {
	reader := bufio.NewReader(bytes.NewReader(append(bytes.Repeat([]byte{'x'}, maximumProtocolLineBytes), '\n')))
	if _, err := readProtocolLine(reader); err == nil {
		t.Fatal("oversized protocol line was accepted")
	}
}

func TestProtocolClientWritesRequestIDsInAllocationOrder(t *testing.T) {
	transport := &recordingTransport{secondWritten: make(chan struct{})}
	firstAllocated := make(chan struct{})
releaseFirst := make(chan struct{})
	client := &protocolClient{
		connection: transport,
		pending: make(map[uint64]chan protocolReply),
		afterAllocate: func(requestID uint64) {
			if requestID == 1 {
				close(firstAllocated)
				<-releaseFirst
			}
		},
	}
	transport.reply = client.finish
	var group sync.WaitGroup
	group.Add(2)
	for range 2 {
		go func() {
			defer group.Done()
			if _, err := client.call(context.Background(), protocolMessage{Type: "post"}); err != nil {
				t.Errorf("call: %v", err)
			}
		}()
	}
	<-firstAllocated
	select {
	case <-transport.secondWritten:
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseFirst)
	group.Wait()
	if got := transport.requestIDs(); !equalRequestIDs(got, []uint64{1, 2}) {
		t.Fatalf("write order = %v, want [1 2]", got)
	}
}

type recordingTransport struct {
	mu sync.Mutex
	writes []uint64
	reply func(uint64, protocolReply)
	secondWritten chan struct{}
}

func (transport *recordingTransport) Read([]byte) (int, error) { return 0, io.EOF }
func (transport *recordingTransport) Close() error { return nil }
func (transport *recordingTransport) Write(data []byte) (int, error) {
	var message protocolMessage
	if err := json.Unmarshal(bytes.TrimSpace(data), &message); err != nil { return 0, err }
	transport.mu.Lock()
	transport.writes = append(transport.writes, message.RequestID)
	if message.RequestID == 2 {
		select { case <-transport.secondWritten: default: close(transport.secondWritten) }
	}
	transport.mu.Unlock()
	transport.reply(message.RequestID, protocolReply{message: protocolMessage{Type: "result"}})
	return len(data), nil
}

func (transport *recordingTransport) requestIDs() []uint64 {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	return append([]uint64(nil), transport.writes...)
}

func equalRequestIDs(got, want []uint64) bool {
	if len(got) != len(want) { return false }
	for index := range got {
		if got[index] != want[index] { return false }
	}
	return true
}
