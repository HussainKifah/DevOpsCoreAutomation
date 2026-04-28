package shell

import (
	"net"
	"os"
	"slices"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
)

// JoinSSHAddr returns host:port for ssh.Dial. If host already includes a port (e.g. 10.0.0.1:2222), it is preserved; otherwise :22 is appended.
func JoinSSHAddr(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if h, port, err := net.SplitHostPort(host); err == nil {
		return net.JoinHostPort(h, port)
	}
	return net.JoinHostPort(host, "22")
}

// wideSSHConfig returns KEX/cipher/MAC lists that include both modern algorithms and legacy
// ones implemented by golang.org/x/crypto/ssh (InsecureAlgorithms). Many Nokia SR / old routers
// only advertise diffie-hellman-group-exchange-sha256 or SHA1 KEX and CBC ciphers; the default
// crypto/ssh client list intentionally omits some of these, which produces "no common algorithm"
// handshake errors.
//
// scrapligo's standard transport sets ClientConfig.Config.KeyExchanges/Ciphers by appending
// "extra" slices onto nil, which replaces the default list entirely—so scrapligo callers must
// pass a full merged list, not only legacy entries.
func wideSSHConfig() ssh.Config {
	s := ssh.SupportedAlgorithms()
	i := ssh.InsecureAlgorithms()
	return ssh.Config{
		KeyExchanges: dedupeStrs(append(slices.Clone(s.KeyExchanges), i.KeyExchanges...)),
		Ciphers:      dedupeStrs(append(slices.Clone(s.Ciphers), i.Ciphers...)),
		MACs:         dedupeStrs(append(slices.Clone(s.MACs), i.MACs...)),
	}
}

func dedupeStrs(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, x := range in {
		if _, ok := seen[x]; ok {
			continue
		}
		seen[x] = struct{}{}
		out = append(out, x)
	}
	return out
}

// scrapligoWideKEX / scrapligoWideCiphers are full algorithm lists for scrapligo's standard
// transport "extra" options (see package comment on wideSSHConfig).
var scrapligoWideKEX, scrapligoWideCiphers, wideSSHHostKeys []string

func wideSSHHostKeyAlgorithms() []string {
	return slices.Clone(wideSSHHostKeys)
}

func init() {
	c := wideSSHConfig()
	scrapligoWideKEX = c.KeyExchanges
	scrapligoWideCiphers = c.Ciphers
	s := ssh.SupportedAlgorithms()
	i := ssh.InsecureAlgorithms()
	wideSSHHostKeys = dedupeStrs(append(slices.Clone(s.HostKeys), i.HostKeys...))
}

var weakRSAHostKeyMu sync.Mutex

func withWeakRSAHostKeySupport(fn func() error) error {
	weakRSAHostKeyMu.Lock()
	defer weakRSAHostKeyMu.Unlock()

	old, hadOld := os.LookupEnv("GODEBUG")
	if old == "" {
		_ = os.Setenv("GODEBUG", "rsa1024min=0")
	} else {
		_ = os.Setenv("GODEBUG", old+",rsa1024min=0")
	}
	defer func() {
		if hadOld {
			_ = os.Setenv("GODEBUG", old)
			return
		}
		_ = os.Unsetenv("GODEBUG")
	}()

	return fn()
}
