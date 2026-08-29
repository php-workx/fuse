//go:build darwin

package attention

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

type lockMetadata struct { OwnerPID int `json:"owner_pid"`; HelperPID int `json:"helper_pid"` }

type fileOwnerLock struct { path string; file *os.File; metadata lockMetadata }

func newFileOwnerLock(home string) *fileOwnerLock {
	return &fileOwnerLock{path: filepath.Join(home, "Library", "Application Support", "Fuse", "notifications.lock")}
}

func (lock *fileOwnerLock) TryAcquire() (bool, error) {
	if err := os.MkdirAll(filepath.Dir(lock.path), 0o700); err != nil { return false, err }
	fd, err := unix.Open(lock.path, unix.O_RDWR|unix.O_CREAT|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil { return false, err }
	file := os.NewFile(uintptr(fd), lock.path)
	var info unix.Stat_t
	if err := unix.Fstat(fd, &info); err != nil { _ = file.Close(); return false, err }
	if info.Mode&unix.S_IFMT != unix.S_IFREG || info.Uid != uint32(os.Geteuid()) { _ = file.Close(); return false, errors.New("notifier lock must be a regular file owned by this user") }
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		if err == unix.EWOULDBLOCK { return false, nil }
		return false, err
	}
	lock.file = file
	lock.metadata = lockMetadata{}
	if _, err := file.Seek(0, io.SeekStart); err == nil {
		contents, _ := io.ReadAll(io.LimitReader(file, 4096))
		_ = json.Unmarshal(contents, &lock.metadata)
	}
	return true, nil
}

func (lock *fileOwnerLock) PreviousHelperPID() int { return lock.metadata.HelperPID }

func (lock *fileOwnerLock) SetHelperPID(pid int) error {
	if lock.file == nil { return errors.New("notifier lock is not held") }
	lock.metadata = lockMetadata{OwnerPID: os.Getpid(), HelperPID: pid}
	contents, err := json.Marshal(lock.metadata)
	if err != nil { return err }
	if err := lock.file.Truncate(0); err != nil { return err }
	if _, err := lock.file.Seek(0, io.SeekStart); err != nil { return err }
	if _, err := lock.file.Write(contents); err != nil { return err }
	return lock.file.Sync()
}

func (lock *fileOwnerLock) Release() error {
	if lock.file == nil { return nil }
	file := lock.file
	lock.file = nil
	if err := unix.Flock(int(file.Fd()), unix.LOCK_UN); err != nil { _ = file.Close(); return err }
	return file.Close()
}
