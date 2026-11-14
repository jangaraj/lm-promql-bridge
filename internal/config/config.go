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
	Portal         string
	AccessID       string
	AccessKey      string
	BearerToken    string
	ServerPort     string
	ServerUsername string
	ServerPassword string
	RequestTimeout time.Duration
	Debug          bool
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

	accessID := os.Getenv("LM_ACCESS_ID")
	accessKey := os.Getenv("LM_ACCESS_KEY")
	bearerToken := os.Getenv("LM_BEARER_TOKEN")

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

	timeoutSecs := 120
	if val := strings.TrimSpace(os.Getenv("REQUEST_TIMEOUT")); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			timeoutSecs = parsed
		}
	}

	debugVal := strings.ToLower(strings.TrimSpace(os.Getenv("DEBUG")))
	debug := debugVal == "true" || debugVal == "1" || debugVal == "yes"

	return &Config{
		Portal:         portal,
		AccessID:       accessID,
		AccessKey:      accessKey,
		BearerToken:    bearerToken,
		ServerPort:     serverPort,
		ServerUsername: serverUsername,
		ServerPassword: serverPassword,
		RequestTimeout: time.Duration(timeoutSecs) * time.Second,
		Debug:          debug,
	}, nil
}
