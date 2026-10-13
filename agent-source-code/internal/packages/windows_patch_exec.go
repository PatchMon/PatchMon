package packages

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"patchmon-agent/internal/winexec"
	"patchmon-agent/pkg/models"
)

const childWaitDelay = 10 * time.Second

type wuaScriptBackend struct{}

func newWUAScriptBackend() wuaBackend { return wuaScriptBackend{} }

type wuaRequest struct {
	Op  string   `json:"op"`
	ID  string   `json:"id,omitempty"`
	IDs []string `json:"ids,omitempty"`
}

func (wuaScriptBackend) exec(ctx context.Context, req wuaRequest, log func(string)) (wuaOutput, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return wuaOutput{}, err
	}
	cmd := exec.CommandContext(ctx, winexec.PowerShellPath(), wuaPowerShellArgs()...)
	cmd.Stdin = bytes.NewReader(payload)
	hideWindow(cmd)
	cmd.WaitDelay = childWaitDelay
	pr, pw := io.Pipe()
	var stderr bytes.Buffer
	cmd.Stdout = pw
	cmd.Stderr = &cappedBuffer{buf: &stderr, limit: 64 * 1024}
	if err := cmd.Start(); err != nil {
		_ = pw.Close()
		return wuaOutput{}, fmt.Errorf("start PowerShell: %w", err)
	}
	type parsed struct {
		out wuaOutput
		err error
	}
	done := make(chan parsed, 1)
	go func() {
		out, perr := parseWUAOutput(pr, log)
		_, _ = io.Copy(io.Discard, pr)
		done <- parsed{out, perr}
	}()
	_ = cmd.Wait()
	_ = pw.Close()
	p := <-done
	if ctx.Err() != nil {
		return p.out, ctx.Err()
	}
	if p.err != nil && stderr.Len() > 0 {
		log(stripTerminalControls(stderr.String()) + "\n")
	}
	return p.out, p.err
}

func (b wuaScriptBackend) Scan(ctx context.Context, log func(string)) (wuaScan, error) {
	out, err := b.exec(ctx, wuaRequest{Op: "scan"}, log)
	if err != nil {
		return wuaScan{}, err
	}
	return wuaScan{Updates: out.Updates, RebootRequiredBeforeInstall: out.Final.RebootRequiredBeforeInstall}, nil
}

func (b wuaScriptBackend) Download(ctx context.Context, ids []string, log func(string)) (map[string]wuaOpResult, error) {
	out, err := b.exec(ctx, wuaRequest{Op: "download", IDs: ids}, log)
	if err != nil {
		return nil, err
	}
	res := make(map[string]wuaOpResult, len(out.Results))
	for _, r := range out.Results {
		res[strings.ToLower(r.ID)] = r
	}
	return res, nil
}

func (b wuaScriptBackend) Install(ctx context.Context, id string, log func(string)) (wuaOpResult, error) {
	out, err := b.exec(ctx, wuaRequest{Op: "install", ID: id}, log)
	if err != nil {
		return wuaOpResult{}, err
	}
	return out.Final.wuaOpResult, nil
}

type wingetExecBackend struct {
	apps func() []models.Package
	path string
	env  []string
}

func newWingetExecBackend(apps func() []models.Package) wingetBackend {
	return &wingetExecBackend{apps: apps}
}

func (b *wingetExecBackend) Resolve() error {
	path, env, err := resolveWinget()
	if err != nil {
		return err
	}
	b.path, b.env = path, env
	return nil
}

func (b *wingetExecBackend) Run(ctx context.Context, args []string, log func(string)) (uint32, string, error) {
	cmd := exec.CommandContext(ctx, b.path, args...)
	cmd.Env = b.env
	hideWindow(cmd)
	cmd.WaitDelay = childWaitDelay
	lines := newLineLogger(log, 1<<20)
	w := &lockedWriter{w: lines}
	cmd.Stdout = w
	cmd.Stderr = w
	err := cmd.Run()
	lines.Flush()
	if ctx.Err() != nil {
		return 0, lines.Output(), ctx.Err()
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return uint32(ee.ExitCode()), lines.Output(), nil
	}
	if err != nil {
		return 0, lines.Output(), err
	}
	return 0, lines.Output(), nil
}

func (b *wingetExecBackend) UpdatableApps() []models.Package {
	if b.apps == nil {
		return nil
	}
	return b.apps()
}

func (b *wingetExecBackend) PendingFileRenames() []string {
	return pendingFileRenames()
}
