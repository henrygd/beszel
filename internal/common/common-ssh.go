package common

var (
	// Allowed ssh key exchanges
	DefaultKeyExchanges = []string{"curve25519-sha256"}
	// Allowed ssh macs
	DefaultMACs = []string{"hmac-sha2-256-etm@openssh.com"}
	// Allowed ssh ciphers
	DefaultCiphers = []string{"chacha20-poly1305@openssh.com"}
)
