package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKubeConfigFromEnv(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_ACCOUNT_TOKEN", "")
	t.Setenv("KUBERNETES_SERVICE_ACCOUNT_TOKEN_FILE", "")
	t.Setenv("KUBERNETES_SERVER", "")
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")
	t.Setenv("KUBERNETES_CA_FILE", "")
	t.Setenv("KUBERNETES_CA_DATA", "")
	t.Setenv("KUBERNETES_INSECURE_SKIP_TLS_VERIFY", "")

	if _, err := kubeConfigFromEnv(); err == nil {
		t.Fatal("expected an error without a token")
	}

	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenFile, []byte("  host-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	caFile := filepath.Join(dir, "ca.crt")
	if err := os.WriteFile(caFile, []byte("ca"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("KUBERNETES_SERVICE_ACCOUNT_TOKEN_FILE", tokenFile)
	t.Setenv("KUBERNETES_SERVICE_HOST", "127.0.0.1")
	t.Setenv("KUBERNETES_SERVICE_PORT", "6443")
	t.Setenv("KUBERNETES_CA_FILE", caFile)

	config, err := kubeConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if config.BearerToken != "host-token" {
		t.Fatalf("token = %q", config.BearerToken)
	}
	if config.Host != "https://127.0.0.1:6443" {
		t.Fatalf("host = %q", config.Host)
	}
	if config.CAFile != caFile {
		t.Fatalf("ca file = %q", config.CAFile)
	}

	t.Setenv("KUBERNETES_SERVICE_ACCOUNT_TOKEN", "inline-token")
	t.Setenv("KUBERNETES_SERVER", "https://kubernetes.default.svc")
	t.Setenv("KUBERNETES_CA_FILE", "")
	t.Setenv("KUBERNETES_INSECURE_SKIP_TLS_VERIFY", "true")

	config, err = kubeConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if config.BearerToken != "inline-token" || !config.Insecure || config.Host != "https://kubernetes.default.svc" {
		t.Fatalf("unexpected config: host=%s insecure=%v", config.Host, config.Insecure)
	}
}
