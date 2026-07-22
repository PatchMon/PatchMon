// Package sshbastion relays multiplexed PTY and raw SSH tunnel messages to agents.
package sshbastion

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"sync"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agentregistry"
)

type Message struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id"`
	Data      string `json:"data,omitempty"`
	Message   string `json:"message,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Sequence  uint64 `json:"sequence,omitempty"`
	Echo      bool   `json:"echo,omitempty"`
}

type Callback func(Message)

type session struct {
	apiID    string
	callback Callback
}

type Broker struct {
	registry *agentregistry.Registry
	mu       sync.RWMutex
	sessions map[string]session
}

func NewBroker(registry *agentregistry.Registry) *Broker {
	return &Broker{registry: registry, sessions: make(map[string]session)}
}

func (b *Broker) Register(sessionID, apiID string, callback Callback) error {
	if sessionID == "" || apiID == "" || callback == nil {
		return errors.New("invalid bastion session registration")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, exists := b.sessions[sessionID]; exists {
		return errors.New("bastion session already exists")
	}
	b.sessions[sessionID] = session{apiID: apiID, callback: callback}
	return nil
}

func (b *Broker) Unregister(sessionID string) {
	b.mu.Lock()
	delete(b.sessions, sessionID)
	b.mu.Unlock()
}

func (b *Broker) HandleAgentMessage(apiID string, raw []byte) {
	var message Message
	if json.Unmarshal(raw, &message) != nil || message.SessionID == "" {
		return
	}
	b.mu.RLock()
	session, exists := b.sessions[message.SessionID]
	b.mu.RUnlock()
	if !exists || session.apiID != apiID {
		return
	}
	session.callback(message)
	if message.Type == "pty_closed" || message.Type == "ssh_tunnel_closed" || message.Type == "pty_error" || message.Type == "ssh_tunnel_error" {
		b.Unregister(message.SessionID)
	}
}

func (b *Broker) OpenPTY(apiID, sessionID, username, terminal string, cols, rows int, callback Callback) error {
	if err := b.Register(sessionID, apiID, callback); err != nil {
		return err
	}
	err := b.registry.SendJSON(apiID, map[string]interface{}{
		"type": "pty_open", "session_id": sessionID, "username": username,
		"terminal": terminal, "cols": cols, "rows": rows,
	})
	if err != nil {
		b.Unregister(sessionID)
	}
	return err
}

func (b *Broker) PTYInput(apiID, sessionID string, sequence uint64, data []byte) error {
	return b.registry.SendJSON(apiID, map[string]interface{}{
		"type": "pty_input", "session_id": sessionID, "sequence": sequence,
		"data": base64.StdEncoding.EncodeToString(data),
	})
}

func (b *Broker) PTYResize(apiID, sessionID string, cols, rows int) error {
	return b.registry.SendJSON(apiID, map[string]interface{}{
		"type": "pty_resize", "session_id": sessionID, "cols": cols, "rows": rows,
	})
}

func (b *Broker) PTYSignal(apiID, sessionID, signal string) error {
	return b.registry.SendJSON(apiID, map[string]interface{}{
		"type": "pty_signal", "session_id": sessionID, "signal": signal,
	})
}

func (b *Broker) ClosePTY(apiID, sessionID string) error {
	b.Unregister(sessionID)
	return b.registry.SendJSON(apiID, map[string]interface{}{"type": "pty_close", "session_id": sessionID})
}

func (b *Broker) OpenTunnel(apiID, sessionID string, callback Callback) error {
	if err := b.Register(sessionID, apiID, callback); err != nil {
		return err
	}
	err := b.registry.SendJSON(apiID, map[string]interface{}{
		"type": "ssh_tunnel_open", "session_id": sessionID, "port": 22,
	})
	if err != nil {
		b.Unregister(sessionID)
	}
	return err
}

func (b *Broker) TunnelInput(apiID, sessionID string, data []byte) error {
	return b.registry.SendJSON(apiID, map[string]interface{}{
		"type": "ssh_tunnel_input", "session_id": sessionID,
		"data": base64.StdEncoding.EncodeToString(data),
	})
}

func (b *Broker) CloseTunnel(apiID, sessionID string) error {
	b.Unregister(sessionID)
	return b.registry.SendJSON(apiID, map[string]interface{}{"type": "ssh_tunnel_close", "session_id": sessionID})
}

func DecodeData(message Message) ([]byte, error) {
	return base64.StdEncoding.DecodeString(message.Data)
}
