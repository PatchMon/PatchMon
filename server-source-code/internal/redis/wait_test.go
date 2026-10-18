package redis

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"testing"
)

func TestIsPermanentError(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("NOAUTH Authentication required."), true},
		{errors.New("WRONGPASS invalid username-password pair or user is disabled."), true},
		{errors.New("ERR invalid password"), true},
		{errors.New("ERR AUTH <password> called without any password configured for the default user"), true},
		{fmt.Errorf("ping: %w", errors.New("WRONGPASS invalid username-password pair")), true},
		{errors.New("NOPERM User default has no permissions to run the 'ping' command"), true},
		{x509.CertificateInvalidError{Reason: x509.Expired}, true},
		{&tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}, true},
		{fmt.Errorf("dial: %w", x509.HostnameError{Host: "redis"}), true},
		{errors.Join(errors.New("dial tcp: i/o timeout"), errors.New("WRONGPASS invalid username-password pair")), true},
		{errors.Join(errors.New("dial tcp: i/o timeout"), &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}), true},
		{errors.Join(errors.New("dial tcp: i/o timeout"), errors.New("connection reset by peer")), false},
		{errors.New("dial tcp 127.0.0.1:6379: connect: connection refused"), false},
		{errors.New("dial tcp: lookup redis on 127.0.0.11:53: no such host"), false},
		{errors.New("LOADING Redis is loading the dataset in memory"), false},
		{errors.New("i/o timeout"), false},
	}
	for _, c := range cases {
		if got := IsPermanentError(c.err); got != c.want {
			t.Errorf("IsPermanentError(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}
