package config

import (
	"strings"
	"testing"
	"time"
)

func setValidEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("LM_PORTAL", "example.logicmonitor.com")
	t.Setenv("LM_ACCESS_ID", "id")
	t.Setenv("LM_ACCESS_KEY", "key")
	t.Setenv("LM_BEARER_TOKEN", "")
	t.Setenv("SERVER_PORT", "")
	t.Setenv("SERVER_USERNAME", "")
	t.Setenv("SERVER_PASSWORD", "")
	t.Setenv("SERVER_TLS_ENABLE", "")
	t.Setenv("SERVER_TLS_CERT", "")
	t.Setenv("SERVER_TLS_KEY", "")
	t.Setenv("REQUEST_TIMEOUT", "")
	t.Setenv("DEBUG", "")
}

func TestLoad_RequiresPortal(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("LM_PORTAL", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when LM_PORTAL is empty")
	}
}

func TestLoad_RequiresBearerOrCredentials(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("LM_ACCESS_ID", "")
	t.Setenv("LM_ACCESS_KEY", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when no auth credentials are provided")
	}
}

func TestLoad_RequiresServerCredentialsTogether(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("SERVER_USERNAME", "user")
	t.Setenv("SERVER_PASSWORD", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when only SERVER_USERNAME is set")
	}
}

func TestLoad_AllowsBasicAuth(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("SERVER_USERNAME", "user")
	t.Setenv("SERVER_PASSWORD", "pass")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if !cfg.BasicAuthEnabled() {
		t.Fatal("expected basic auth to be enabled")
	}
	if cfg.ServerUsername != "user" {
		t.Fatalf("expected username user, got %q", cfg.ServerUsername)
	}
	if cfg.ServerPassword != "pass" {
		t.Fatalf("expected password pass, got %q", cfg.ServerPassword)
	}
}

func TestLoad_DisablesBasicAuthWhenUnset(t *testing.T) {
	setValidEnvironment(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.BasicAuthEnabled() {
		t.Fatal("expected basic auth to be disabled")
	}
}

func TestLoad_Defaults(t *testing.T) {
	setValidEnvironment(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.ServerPort != "8080" {
		t.Errorf("expected default server port 8080, got %q", cfg.ServerPort)
	}
	if cfg.ServerTLSEnabled {
		t.Error("expected TLS to be disabled by default")
	}
	if cfg.RequestTimeout != 120*time.Second {
		t.Errorf("expected default request timeout 120s, got %s", cfg.RequestTimeout)
	}
	if cfg.Debug {
		t.Error("expected debug to be disabled by default")
	}
}

func TestLoad_RequestTimeout(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    time.Duration
		wantErr bool
	}{
		{name: "positive integer", value: "45", want: 45 * time.Second},
		{name: "not an integer", value: "abc", wantErr: true},
		{name: "fractional", value: "1.5", wantErr: true},
		{name: "zero", value: "0", wantErr: true},
		{name: "negative", value: "-10", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setValidEnvironment(t)
			t.Setenv("REQUEST_TIMEOUT", tt.value)

			cfg, err := Load()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected invalid REQUEST_TIMEOUT to fail")
				}
				if !strings.Contains(err.Error(), "REQUEST_TIMEOUT") {
					t.Fatalf("expected error to mention REQUEST_TIMEOUT, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() returned error: %v", err)
			}
			if cfg.RequestTimeout != tt.want {
				t.Errorf("expected timeout %s, got %s", tt.want, cfg.RequestTimeout)
			}
		})
	}
}

func TestLoad_BooleanEnvironmentVariables(t *testing.T) {
	tests := []struct {
		name      string
		env       string
		value     string
		want      bool
		wantError bool
	}{
		{name: "TLS true", env: "SERVER_TLS_ENABLE", value: "true", want: true},
		{name: "TLS false", env: "SERVER_TLS_ENABLE", value: "false", want: false},
		{name: "debug yes", env: "DEBUG", value: "yes", want: true},
		{name: "debug no", env: "DEBUG", value: "no", want: false},
		{name: "invalid TLS boolean", env: "SERVER_TLS_ENABLE", value: "tru", wantError: true},
		{name: "invalid debug boolean", env: "DEBUG", value: "sometimes", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setValidEnvironment(t)
			t.Setenv(tt.env, tt.value)

			cfg, err := Load()
			if tt.wantError {
				if err == nil {
					t.Fatal("expected malformed boolean environment value to fail")
				}
				if !strings.Contains(err.Error(), tt.env) {
					t.Fatalf("expected error to mention %s, got %v", tt.env, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() returned error: %v", err)
			}

			var got bool
			if tt.env == "SERVER_TLS_ENABLE" {
				got = cfg.ServerTLSEnabled
			} else {
				got = cfg.Debug
			}

			if got != tt.want {
				t.Errorf("expected %s=%q to parse as %t", tt.env, tt.value, tt.want)
			}
		})
	}
}

func TestLoad_TLSCertificateAndKeyMustBePaired(t *testing.T) {
	tests := []struct {
		name string
		cert string
		key  string
	}{
		{name: "certificate only", cert: "/tmp/server.crt"},
		{name: "key only", key: "/tmp/server.key"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setValidEnvironment(t)
			t.Setenv("SERVER_TLS_ENABLE", "true")
			t.Setenv("SERVER_TLS_CERT", tt.cert)
			t.Setenv("SERVER_TLS_KEY", tt.key)

			_, err := Load()
			if err == nil {
				t.Fatal("expected error when only one TLS file is configured")
			}
			if !strings.Contains(err.Error(), "SERVER_TLS_CERT") || !strings.Contains(err.Error(), "SERVER_TLS_KEY") {
				t.Fatalf("expected error to mention both TLS variables, got %v", err)
			}
		})
	}
}

func TestLoad_TLSAllowsSelfSignedCertificateWhenFilesOmitted(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("SERVER_TLS_ENABLE", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if !cfg.ServerTLSEnabled {
		t.Fatal("expected TLS to be enabled")
	}
	if cfg.ServerTLSCert != "" || cfg.ServerTLSKey != "" {
		t.Fatal("expected TLS certificate and key paths to be empty for self-signed mode")
	}
}
