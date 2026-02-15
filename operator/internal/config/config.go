package config

import (
	"os"
	"strconv"
	"strings"
)

// ServerConfig holds the configuration for the gRPC server.
type ServerConfig struct {
	// Port is the port the gRPC server listens on.
	Port int
	// HTTPPort is the port the HTTP/REST gateway listens on.
	HTTPPort int
	// MetricsPort is the port for the metrics server.
	MetricsPort int
	// HealthPort is the port for the health check server.
	HealthPort int
	// Auth holds OIDC authentication configuration.
	Auth AuthConfig
}

// AuthConfig holds OIDC authentication settings.
type AuthConfig struct {
	// Enabled controls whether authentication is enforced.
	Enabled bool
	// IssuerURL is the Dex OIDC issuer URL (e.g. https://dex.example.com).
	IssuerURL string
	// ClientID is the OIDC client ID that tokens must be issued for.
	ClientID string
	// AllowedOrigins is a comma-separated list of allowed CORS origins.
	// Defaults to "*" if not set.
	AllowedOrigins []string
}

// LoadServerConfig loads the server configuration from environment variables.
func LoadServerConfig() *ServerConfig {
	return &ServerConfig{
		Port:        getEnvInt("GRPC_PORT", 50051),
		HTTPPort:    getEnvInt("HTTP_PORT", 8080),
		MetricsPort: getEnvInt("METRICS_PORT", 9090),
		HealthPort:  getEnvInt("HEALTH_PORT", 8081),
		Auth: AuthConfig{
			Enabled:        getEnvBool("AUTH_ENABLED", false),
			IssuerURL:      getEnvStr("AUTH_OIDC_ISSUER_URL", ""),
			ClientID:       getEnvStr("AUTH_OIDC_CLIENT_ID", "flokoa"),
			AllowedOrigins: getEnvList("AUTH_CORS_ALLOWED_ORIGINS", []string{"*"}),
		},
	}
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil {
			return intVal
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		if boolVal, err := strconv.ParseBool(val); err == nil {
			return boolVal
		}
	}
	return defaultVal
}

func getEnvStr(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvList(key string, defaultVal []string) []string {
	if val := os.Getenv(key); val != "" {
		parts := strings.Split(val, ",")
		result := make([]string, 0, len(parts))
		for _, p := range parts {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				result = append(result, trimmed)
			}
		}
		return result
	}
	return defaultVal
}
