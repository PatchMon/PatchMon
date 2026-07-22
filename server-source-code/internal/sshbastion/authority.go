package sshbastion

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

const CertificateTTL = 5 * time.Minute

type Authority struct {
	signer  ssh.Signer
	trusted []ssh.PublicKey
}

func LoadAuthority(privateKeyFile, passphraseFile string, previousCAPublicKeys []string) (*Authority, error) {
	data, err := os.ReadFile(privateKeyFile)
	if err != nil {
		return nil, fmt.Errorf("read SSH CA key: %w", err)
	}
	passphrase, passphraseErr := os.ReadFile(passphraseFile)
	if passphraseErr != nil {
		for i := range data {
			data[i] = 0
		}
		return nil, fmt.Errorf("read SSH CA passphrase: %w", passphraseErr)
	}
	signer, err := ssh.ParsePrivateKeyWithPassphrase(data, bytes.TrimSpace(passphrase))
	for i := range data {
		data[i] = 0
	}
	for i := range passphrase {
		passphrase[i] = 0
	}
	if err != nil {
		return nil, fmt.Errorf("parse SSH CA key: %w", err)
	}
	authority := &Authority{signer: signer, trusted: []ssh.PublicKey{signer.PublicKey()}}
	for _, encoded := range previousCAPublicKeys {
		if strings.TrimSpace(encoded) == "" {
			continue
		}
		key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(encoded))
		if err != nil {
			return nil, fmt.Errorf("parse previous SSH CA public key: %w", err)
		}
		authority.trusted = append(authority.trusted, key)
	}
	return authority, nil
}

type CertificateClaims struct {
	UserID, HostID, Tenant, LinuxUsername string
}

func (a *Authority) Sign(publicKey ssh.PublicKey, claims CertificateClaims, now time.Time) (*ssh.Certificate, error) {
	if publicKey == nil || claims.UserID == "" || claims.HostID == "" || claims.LinuxUsername == "" {
		return nil, errors.New("incomplete SSH certificate request")
	}
	serialBytes := make([]byte, 8)
	if _, err := rand.Read(serialBytes); err != nil {
		return nil, err
	}
	cert := &ssh.Certificate{
		Key:             publicKey,
		Serial:          uint64(serialBytes[0])<<56 | uint64(serialBytes[1])<<48 | uint64(serialBytes[2])<<40 | uint64(serialBytes[3])<<32 | uint64(serialBytes[4])<<24 | uint64(serialBytes[5])<<16 | uint64(serialBytes[6])<<8 | uint64(serialBytes[7]),
		CertType:        ssh.UserCert,
		KeyId:           claims.UserID + ":" + claims.HostID,
		ValidAfter:      uint64(now.Add(-15 * time.Second).Unix()),
		ValidBefore:     uint64(now.Add(CertificateTTL).Unix()),
		ValidPrincipals: []string{claims.LinuxUsername},
		Permissions: ssh.Permissions{Extensions: map[string]string{
			"patchmon-user-id": claims.UserID, "patchmon-host-id": claims.HostID,
			"patchmon-tenant": claims.Tenant, "permit-pty": "",
		}},
	}
	if err := cert.SignCert(rand.Reader, a.signer); err != nil {
		return nil, fmt.Errorf("sign SSH certificate: %w", err)
	}
	return cert, nil
}

func (a *Authority) Check(cert *ssh.Certificate, principal string, now time.Time) error {
	if cert == nil || cert.CertType != ssh.UserCert {
		return errors.New("an SSH user certificate is required")
	}
	checker := ssh.CertChecker{
		Clock: func() time.Time { return now },
		IsUserAuthority: func(key ssh.PublicKey) bool {
			for _, trusted := range a.trusted {
				if string(key.Marshal()) == string(trusted.Marshal()) {
					return true
				}
			}
			return false
		},
	}
	return checker.CheckCert(principal, cert)
}

func (a *Authority) PublicKey() string {
	return string(ssh.MarshalAuthorizedKey(a.signer.PublicKey()))
}

func EncodeCertificate(cert *ssh.Certificate) string {
	return base64.StdEncoding.EncodeToString(cert.Marshal())
}
