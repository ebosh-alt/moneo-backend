package bootstrap

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const (
	envPostgresURL      = "POSTGRES_URL"
	envPostgresHost     = "POSTGRES_HOST"
	envPostgresPort     = "POSTGRES_PORT"
	envPostgresUser     = "POSTGRES_USER"
	envPostgresPassword = "POSTGRES_PASSWORD"
	envPostgresDBName   = "POSTGRES_DBNAME"
	envPostgresSSLMode  = "POSTGRES_SSLMODE"
)

type PostgresConfig struct {
	DSN string
}

func LoadPostgresConfigFromEnv() (PostgresConfig, error) {
	rawURL := strings.TrimSpace(os.Getenv(envPostgresURL))
	if rawURL != "" {
		return PostgresConfig{DSN: rawURL}, nil
	}

	host := strings.TrimSpace(os.Getenv(envPostgresHost))
	if host == "" {
		return PostgresConfig{}, fmt.Errorf("%s is required when %s is not set", envPostgresHost, envPostgresURL)
	}

	user := strings.TrimSpace(os.Getenv(envPostgresUser))
	if user == "" {
		return PostgresConfig{}, fmt.Errorf("%s is required when %s is not set", envPostgresUser, envPostgresURL)
	}

	password := strings.TrimSpace(os.Getenv(envPostgresPassword))
	if password == "" {
		return PostgresConfig{}, fmt.Errorf("%s is required when %s is not set", envPostgresPassword, envPostgresURL)
	}

	dbName := strings.TrimSpace(os.Getenv(envPostgresDBName))
	if dbName == "" {
		return PostgresConfig{}, fmt.Errorf("%s is required when %s is not set", envPostgresDBName, envPostgresURL)
	}

	port := strings.TrimSpace(os.Getenv(envPostgresPort))
	if port == "" {
		port = "5432"
	}
	if _, err := strconv.Atoi(port); err != nil {
		return PostgresConfig{}, fmt.Errorf("invalid %s %q: %w", envPostgresPort, port, err)
	}

	sslmode := strings.TrimSpace(os.Getenv(envPostgresSSLMode))
	if sslmode == "" {
		sslmode = "disable"
	}

	postgresURL := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   host + ":" + port,
		Path:   dbName,
	}
	query := postgresURL.Query()
	query.Set("sslmode", sslmode)
	postgresURL.RawQuery = query.Encode()

	return PostgresConfig{DSN: postgresURL.String()}, nil
}
