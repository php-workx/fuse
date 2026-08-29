//go:build darwin

package attention

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

type launchServicesStarter struct { app string }

type launchTransport struct { net.Conn; listener *net.UnixListener; directory string }
type openProcess struct { command *exec.Cmd }

func (starter launchServicesStarter) Start(ctx context.Context, _ TerminalTargetMetadata) (io.ReadWriteCloser, helperProcess, error) {
	directory, err := os.MkdirTemp("", "fuse-notifier-")
	if err != nil { return nil, nil, err }
	cleanup := func() { _ = os.RemoveAll(directory) }
	if err := os.Chmod(directory, 0o700); err != nil { cleanup(); return nil, nil, err }
	path := filepath.Join(directory, "notifier.sock")
	if len(path) >= 104 { cleanup(); return nil, nil, errors.New("notifier socket path is too long") }
	address := &net.UnixAddr{Name: path, Net: "unix"}
	listener, err := net.ListenUnix("unix", address)
	if err != nil { cleanup(); return nil, nil, err }
	if err := os.Chmod(path, 0o600); err != nil { _ = listener.Close(); cleanup(); return nil, nil, err }
	tokenBytes := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, tokenBytes); err != nil { _ = listener.Close(); cleanup(); return nil, nil, err }
	token := hex.EncodeToString(tokenBytes)
	command := exec.CommandContext(ctx, "/usr/bin/open", "-n", "-g", starter.app, "--args", "--connect", path, "--token", token)
	if err := command.Start(); err != nil { _ = listener.Close(); cleanup(); return nil, nil, err }
	if err := listener.SetDeadline(time.Now().Add(10 * time.Second)); err != nil { _ = listener.Close(); cleanup(); return nil, nil, err }
	connection, err := listener.AcceptUnix()
	if err != nil { _ = listener.Close(); cleanup(); return nil, nil, err }
	if err := verifyPeerUID(connection); err != nil { _ = connection.Close(); _ = listener.Close(); cleanup(); return nil, nil, err }
	if err := authenticateSocket(connection, token); err != nil { _ = connection.Close(); _ = listener.Close(); cleanup(); return nil, nil, err }
	return &launchTransport{Conn: connection, listener: listener, directory: directory}, openProcess{command: command}, nil
}

func (transport *launchTransport) Close() error {
	connectionError := transport.Conn.Close()
	listenerError := transport.listener.Close()
	removeError := os.RemoveAll(transport.directory)
	if connectionError != nil { return connectionError }
	if listenerError != nil { return listenerError }
	return removeError
}

func (process openProcess) PID() int { return process.command.Process.Pid }
func (process openProcess) Wait() error { return process.command.Wait() }

func verifyPeerUID(connection *net.UnixConn) error {
	raw, err := connection.SyscallConn()
	if err != nil { return err }
	var peerUID uint32
	var controlError error
	err = raw.Control(func(fd uintptr) {
		credentials, credentialError := unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
		if credentialError != nil { controlError = credentialError; return }
		peerUID = credentials.Uid
	})
	if err != nil { return err }
	if controlError != nil { return controlError }
	if peerUID != uint32(os.Geteuid()) { return errors.New("notifier socket peer has a different UID") }
	return nil
}

func authenticateSocket(connection *net.UnixConn, token string) error {
	reader := bufio.NewReader(connection)
	if err := connection.SetDeadline(time.Now().Add(10 * time.Second)); err != nil { return err }
	line, err := reader.ReadString('\n')
	if err != nil { return err }
	if line != "token "+token+"\n" { return errors.New("notifier socket handshake token mismatch") }
	if _, err := connection.Write([]byte("ok\n")); err != nil { return err }
	return connection.SetDeadline(time.Time{})
}
