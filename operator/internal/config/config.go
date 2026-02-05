package config

import (
	"os"
	"strconv"
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
}

// LoadServerConfig loads the server configuration from environment variables.
func LoadServerConfig() *ServerConfig {
	return &ServerConfig{
		Port:        getEnvInt("GRPC_PORT", 50051),
		HTTPPort:    getEnvInt("HTTP_PORT", 8080),
		MetricsPort: getEnvInt("METRICS_PORT", 9090),
		HealthPort:  getEnvInt("HEALTH_PORT", 8081),
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
