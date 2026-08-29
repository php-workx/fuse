package attention

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

const maximumProtocolLineBytes = 32 * 1024
const maximumDisplayScalars = 160

var errProtocolClosed = errors.New("notifier transport closed")
type terminalWire struct {
	TerminalProgram string `json:"terminal_program"`
	TerminalProgramVersion string `json:"terminal_program_version"`
	ITermSessionID string `json:"iterm_session_id,omitempty"`
	TerminalTTY string `json:"terminal_tty,omitempty"`
	GhosttyTitleMarker string `json:"ghostty_title_marker,omitempty"`
	ExactTargetingDegraded bool `json:"exact_targeting_degraded"`
}


type protocolMessage struct {
	Version int `json:"version"`
	Type string `json:"type"`
	RequestID uint64 `json:"request_id,omitempty"`
	Operation string `json:"operation,omitempty"`
	ID string `json:"id,omitempty"`
	Status string `json:"status,omitempty"`
	ClientVersion string `json:"client_version,omitempty"`
	HelperVersion string `json:"helper_version,omitempty"`
	ProtocolMin int `json:"protocol_min,omitempty"`
	ProtocolMax int `json:"protocol_max,omitempty"`
	PID int `json:"pid,omitempty"`
	Authorization string `json:"authorization,omitempty"`
	AlertSetting string `json:"alert_setting,omitempty"`
	SoundSetting string `json:"sound_setting,omitempty"`
	Target string `json:"target,omitempty"`
	Title string `json:"title,omitempty"`
	Subtitle string `json:"subtitle,omitempty"`
	Body string `json:"body,omitempty"`
	Sound bool `json:"sound,omitempty"`
	CurrentIDs []string `json:"current_ids,omitempty"`
	DeliveredCurrentIDs []string `json:"delivered_current_ids,omitempty"`
	Mode string `json:"mode,omitempty"`
	Terminal *terminalWire `json:"terminal,omitempty"`
	Detail string `json:"detail,omitempty"`
	Code string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type protocolReply struct {
	message protocolMessage
	err error
}

type protocolClient struct {
	connection io.ReadWriteCloser
	writeMu sync.Mutex
	mu sync.Mutex
	nextRequestID uint64
	pending map[uint64]chan protocolReply
	shutdown chan protocolReply
	onMessage func(protocolMessage)
	onFailure func(error)
	closed bool
	afterAllocate func(uint64)
}

func newProtocolClient(connection io.ReadWriteCloser, onMessage func(protocolMessage), onFailure func(error)) *protocolClient {
	client := &protocolClient{connection: connection, pending: make(map[uint64]chan protocolReply), shutdown: make(chan protocolReply, 1), onMessage: onMessage, onFailure: onFailure}
	go client.read()
	return client
}

func (client *protocolClient) call(ctx context.Context, command protocolMessage) (protocolMessage, error) {
	client.writeMu.Lock()
	client.mu.Lock()
	if client.closed {
		client.mu.Unlock()
		client.writeMu.Unlock()
		return protocolMessage{}, errProtocolClosed
	}
	client.nextRequestID++
	command.Version = 1
	command.RequestID = client.nextRequestID
	response := make(chan protocolReply, 1)
	client.pending[command.RequestID] = response
	client.mu.Unlock()
	if client.afterAllocate != nil { client.afterAllocate(command.RequestID) }
	err := client.writeLocked(command)
	client.writeMu.Unlock()
	if err != nil {
		client.finish(command.RequestID, protocolReply{err: err})
		return protocolMessage{}, err
	}
	select {
	case reply := <-response:
		return reply.message, reply.err
	case <-ctx.Done():
		client.finish(command.RequestID, protocolReply{err: ctx.Err()})
		return protocolMessage{}, ctx.Err()
	}
}

func (client *protocolClient) shutdownRequest(ctx context.Context) error {
	client.writeMu.Lock()
	client.mu.Lock()
	if client.closed {
		client.mu.Unlock()
		client.writeMu.Unlock()
		return errProtocolClosed
	}
	client.nextRequestID++
	command := protocolMessage{Version: 1, Type: "shutdown", RequestID: client.nextRequestID}
	client.mu.Unlock()
	if client.afterAllocate != nil { client.afterAllocate(command.RequestID) }
	err := client.writeLocked(command)
	client.writeMu.Unlock()
	if err != nil { return err }
	select {
	case reply := <-client.shutdown:
		return reply.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (client *protocolClient) writeLocked(message protocolMessage) error {
	line, err := json.Marshal(message)
	if err != nil { return err }
	if len(line)+1 > maximumProtocolLineBytes { return fmt.Errorf("notifier protocol line exceeds %d bytes", maximumProtocolLineBytes) }
	line = append(line, '\n')
	_, err = client.connection.Write(line)
	return err
}

func (client *protocolClient) read() {
	reader := bufio.NewReader(client.connection)
	for {
		line, err := readProtocolLine(reader)
		if err != nil {
			if errors.Is(err, io.EOF) { client.fail(errProtocolClosed) } else { client.fail(err) }
			return
		}
		var message protocolMessage
		if err := json.Unmarshal(line, &message); err != nil || message.Version != 1 || message.Type == "" {
			client.fail(fmt.Errorf("invalid notifier protocol message"))
			return
		}
		if message.Type == "shutdown_complete" {
			select { case client.shutdown <- protocolReply{}: default: }
			continue
		}
		if message.RequestID != 0 && (message.Type == "ready" || message.Type == "result" || message.Type == "reconciled" || message.Type == "focus_result" || message.Type == "error") {
			var reply protocolReply
			if message.Type == "error" { reply = protocolReply{message: message, err: fmt.Errorf("notifier %s: %s", message.Code, message.Message)} } else { reply.message = message }
			client.finish(message.RequestID, reply)
			continue
		}
		client.onMessage(message)
	}
}

func readProtocolLine(reader *bufio.Reader) ([]byte, error) {
	line := make([]byte, 0, 1024)
	for {
		fragment, err := reader.ReadSlice('\n')
		if len(line)+len(fragment) > maximumProtocolLineBytes { return nil, fmt.Errorf("notifier protocol line exceeds %d bytes", maximumProtocolLineBytes) }
		line = append(line, fragment...)
		if err == nil { return line, nil }
		if errors.Is(err, bufio.ErrBufferFull) { continue }
		return nil, err
	}
}

func (client *protocolClient) finish(requestID uint64, reply protocolReply) {
	client.mu.Lock()
	response := client.pending[requestID]
	delete(client.pending, requestID)
	client.mu.Unlock()
	if response != nil { response <- reply }
}

func (client *protocolClient) fail(err error) {
	client.mu.Lock()
	if client.closed { client.mu.Unlock(); return }
	client.closed = true
	pending := client.pending
	client.pending = make(map[uint64]chan protocolReply)
	client.mu.Unlock()
	for _, response := range pending { response <- protocolReply{err: err} }
	client.onFailure(err)
}

func (client *protocolClient) close() error {
	client.mu.Lock()
	client.closed = true
	client.mu.Unlock()
	return client.connection.Close()
}

func trimDisplay(value string) string {
	count := 0
	for index := range value {
		if count == maximumDisplayScalars { return value[:index] }
		count++
	}
	return value
}
