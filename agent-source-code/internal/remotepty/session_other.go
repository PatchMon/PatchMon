//go:build !linux

package remotepty

import (
	"errors"
	"io"
)

type Session struct{}

func Start(string, string, int, int) (*Session, error) {
	return nil, errors.New("local PTY bastion sessions are supported only on Linux")
}
func (*Session) Output() io.Reader               { return nil }
func (*Session) WriteInput([]byte) (bool, error) { return false, errors.New("unsupported") }
func (*Session) Resize(int, int) error           { return errors.New("unsupported") }
func (*Session) Signal(string) error             { return errors.New("unsupported") }
func (*Session) Wait() error                     { return errors.New("unsupported") }
func (*Session) Close() error                    { return nil }
