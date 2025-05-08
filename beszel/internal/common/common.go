package common

var (
	DefaultKeyExchanges = []string{"curve25519-sha256"}
	DefaultMACs         = []string{"hmac-sha2-256-etm@openssh.com"}
	DefaultCiphers      = []string{"chacha20-poly1305@openssh.com"}
)
