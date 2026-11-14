package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Portal           string
	AccessID         string
	AccessKey        string
	BearerToken      string
	ServerPort       string
	ServerUsername   string
	ServerPassword   string
	ServerTLSEnabled bool
	ServerTLSCert    string
	ServerTLSKey     string
	RequestTimeout   time.Duration
	Debug            bool
}

func (c *Config) BasicAuthEnabled() bool {
	return c.ServerUsername != "" && c.ServerPassword != ""
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	portal := strings.TrimSpace(os.Getenv("LM_PORTAL"))
	if portal == "" {
		return nil, fmt.Errorf("LM_PORTAL environment variable is required")
	}
	portal = strings.TrimPrefix(portal, "https://")
	portal = strings.TrimPrefix(portal, "http://")
	portal = strings.TrimSuffix(portal, "/")

	if !strings.Contains(portal, ".") {
		portal = portal + ".logicmonitor.com"
	}

	accessID := strings.TrimSpace(os.Getenv("LM_ACCESS_ID"))
	accessKey := strings.TrimSpace(os.Getenv("LM_ACCESS_KEY"))
	bearerToken := strings.TrimSpace(os.Getenv("LM_BEARER_TOKEN"))

	if bearerToken == "" && (accessID == "" || accessKey == "") {
		return nil, fmt.Errorf("either LM_BEARER_TOKEN or both LM_ACCESS_ID and LM_ACCESS_KEY must be provided")
	}

	serverPort := strings.TrimSpace(os.Getenv("SERVER_PORT"))
	if serverPort == "" {
		serverPort = "8080"
	}

	serverUsername := strings.TrimSpace(os.Getenv("SERVER_USERNAME"))
	serverPassword := strings.TrimSpace(os.Getenv("SERVER_PASSWORD"))

	if (serverUsername == "") != (serverPassword == "") {
		return nil, fmt.Errorf("SERVER_USERNAME and SERVER_PASSWORD must be set together")
	}

	tlsEnabled, err := parseBoolEnv("SERVER_TLS_ENABLE", false)
	if err != nil {
		return nil, err
	}

	serverTLSCert := strings.TrimSpace(os.Getenv("SERVER_TLS_CERT"))
	serverTLSKey := strings.TrimSpace(os.Getenv("SERVER_TLS_KEY"))

	if tlsEnabled && ((serverTLSCert == "") != (serverTLSKey == "")) {
		return nil, fmt.Errorf("SERVER_TLS_CERT and SERVER_TLS_KEY must be set together when TLS is enabled; omit both to generate a self-signed certificate")
	}

	debug, err := parseBoolEnv("DEBUG", false)
	if err != nil {
		return nil, err
	}

	requestTimeout := 120 * time.Second
	if value := strings.TrimSpace(os.Getenv("REQUEST_TIMEOUT")); value != "" {
		seconds, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("REQUEST_TIMEOUT must be a positive integer number of seconds: %w", err)
		}
		if seconds <= 0 {
			return nil, fmt.Errorf("REQUEST_TIMEOUT must be greater than zero")
		}
		requestTimeout = time.Duration(seconds) * time.Second
	}

	return &Config{
		Portal:           portal,
		AccessID:         accessID,
		AccessKey:        accessKey,
		BearerToken:      bearerToken,
		ServerPort:       serverPort,
		ServerUsername:   serverUsername,
		ServerPassword:   serverPassword,
		ServerTLSEnabled: tlsEnabled,
		ServerTLSCert:    serverTLSCert,
		ServerTLSKey:     serverTLSKey,
		RequestTimeout:   requestTimeout,
		Debug:            debug,
	}, nil
}

func parseBoolEnv(name string, defaultValue bool) (bool, error) {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if value == "" {
		return defaultValue, nil
	}

	switch value {
	case "true", "1", "yes":
		return true, nil
	case "false", "0", "no":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be a boolean (true/false, yes/no, or 1/0)", name)
	}
}
