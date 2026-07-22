//go:build linux

package remotepty

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"sync"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

type Session struct {
	cmd    *exec.Cmd
	pty    *os.File
	once   sync.Once
	mu     sync.Mutex
	waitCh chan error
}

func Start(username, terminal string, cols, rows int) (*Session, error) {
	account, err := user.Lookup(username)
	if err != nil {
		return nil, fmt.Errorf("lookup Linux account: %w", err)
	}
	if account.Uid == "0" || username == "root" {
		return nil, errors.New("root login is disabled")
	}
	uid, err := parseID("uid", account.Uid)
	if err != nil {
		return nil, err
	}
	gid, err := parseID("gid", account.Gid)
	if err != nil {
		return nil, err
	}
	groups, err := account.GroupIds()
	if err != nil {
		return nil, fmt.Errorf("resolve supplementary groups: %w", err)
	}
	groupIDs := make([]uint32, 0, len(groups))
	for _, value := range groups {
		id, err := parseID("group id", value)
		if err != nil {
			return nil, err
		}
		groupIDs = append(groupIDs, id)
	}

	shell := account.HomeDir + "/.patchmon-shell"
	if stat, err := os.Stat(shell); err != nil || stat.IsDir() {
		shell = os.Getenv("SHELL")
	}
	if shell == "" {
		shell = "/bin/sh"
	}
	if stat, err := os.Stat(shell); err != nil || stat.IsDir() || stat.Mode()&0111 == 0 {
		return nil, fmt.Errorf("login shell %q is not executable", shell)
	}
	if terminal == "" {
		terminal = "xterm-256color"
	}
	if cols < 1 {
		cols = 80
	}
	if rows < 1 {
		rows = 24
	}
	cmd := exec.Command(shell, "-l")
	cmd.Dir = account.HomeDir
	cmd.Env = []string{
		"HOME=" + account.HomeDir,
		"USER=" + account.Username,
		"LOGNAME=" + account.Username,
		"SHELL=" + shell,
		"TERM=" + terminal,
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{Uid: uid, Gid: gid, Groups: groupIDs},
		Setsid:     true,
		Setctty:    true,
	}
	file, err := pty.StartWithAttrs(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)}, cmd.SysProcAttr)
	if err != nil {
		return nil, fmt.Errorf("start account PTY: %w", err)
	}
	session := &Session{cmd: cmd, pty: file, waitCh: make(chan error, 1)}
	go func() { session.waitCh <- cmd.Wait() }()
	return session, nil
}

func (s *Session) Output() io.Reader { return s.pty }

// WriteInput returns the terminal ECHO state observed immediately before the
// input was written. Callers must only record the input when echo is true.
func (s *Session) WriteInput(data []byte) (echo bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	termios, ioctlErr := unix.IoctlGetTermios(int(s.pty.Fd()), unix.TCGETS)
	if ioctlErr != nil {
		return false, fmt.Errorf("read PTY echo state: %w", ioctlErr)
	}
	echo = termios.Lflag&unix.ECHO != 0
	_, err = s.pty.Write(data)
	return echo, err
}

func (s *Session) Resize(cols, rows int) error {
	if cols < 1 || rows < 1 || cols > 1000 || rows > 1000 {
		return errors.New("invalid terminal dimensions")
	}
	return pty.Setsize(s.pty, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

func (s *Session) Signal(name string) error {
	var signal os.Signal
	switch name {
	case "INT":
		signal = os.Interrupt
	case "TERM":
		signal = syscall.SIGTERM
	case "HUP":
		signal = syscall.SIGHUP
	default:
		return errors.New("unsupported signal")
	}
	if s.cmd.Process == nil {
		return errors.New("session process is not running")
	}
	return s.cmd.Process.Signal(signal)
}

func (s *Session) Wait() error { return <-s.waitCh }

func (s *Session) Close() error {
	var closeErr error
	s.once.Do(func() {
		if s.cmd.Process != nil {
			_ = s.cmd.Process.Signal(syscall.SIGHUP)
		}
		closeErr = s.pty.Close()
	})
	return closeErr
}

func parseID(kind, value string) (uint32, error) {
	id, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", kind, err)
	}
	return uint32(id), nil
}
