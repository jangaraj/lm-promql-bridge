package config

import "testing"

func TestLoad_RequiresPortal(t *testing.T) {
	t.Setenv("LM_PORTAL", "")
	t.Setenv("LM_ACCESS_ID", "id")
	t.Setenv("LM_ACCESS_KEY", "key")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when LM_PORTAL is empty")
	}
}

func TestLoad_RequiresBearerOrCredentials(t *testing.T) {
	t.Setenv("LM_PORTAL", "example.logicmonitor.com")
	t.Setenv("LM_ACCESS_ID", "")
	t.Setenv("LM_ACCESS_KEY", "")
	t.Setenv("LM_BEARER_TOKEN", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when no auth credentials are provided")
	}
}

func TestLoad_RequiresServerCredentialsTogether(t *testing.T) {
	t.Setenv("LM_PORTAL", "example.logicmonitor.com")
	t.Setenv("LM_ACCESS_ID", "id")
	t.Setenv("LM_ACCESS_KEY", "key")
	t.Setenv("SERVER_USERNAME", "user")
	t.Setenv("SERVER_PASSWORD", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when only SERVER_USERNAME is set")
	}
}

func TestLoad_AllowsBasicAuth(t *testing.T) {
	t.Setenv("LM_PORTAL", "example.logicmonitor.com")
	t.Setenv("LM_ACCESS_ID", "id")
	t.Setenv("LM_ACCESS_KEY", "key")
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
	t.Setenv("LM_PORTAL", "example.logicmonitor.com")
	t.Setenv("LM_ACCESS_ID", "id")
	t.Setenv("LM_ACCESS_KEY", "key")
	t.Setenv("SERVER_USERNAME", "")
	t.Setenv("SERVER_PASSWORD", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.BasicAuthEnabled() {
		t.Fatal("expected basic auth to be disabled")
	}
}
