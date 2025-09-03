package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Server   ServerConfig
	Postgres PostgresConfig
	Redis    RedisConfig
}

type ServerConfig struct {
	Host string
	Port int
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type PostgresConfig struct {
	User     string
	Password string
	Name     string
	Host     string
	Port     int
	SSLMode  string
}

func New() (*Config, error) {
	const op = "config.New"

	_ = godotenv.Load()

	serverHost := os.Getenv("SERVER_HOST")
	if serverHost == "" {
		serverHost = "localhost"
	}

	serverPortStr := os.Getenv("SERVER_PORT")
	if serverPortStr == "" {
		serverPortStr = "8080"
	}

	serverPort, err := strconv.Atoi(serverPortStr)
	if err != nil {
		return nil, fmt.Errorf("%s: invalid SERVER_PORT: %w", op, err)
	}

	serverCfg := ServerConfig{
		Host: serverHost,
		Port: serverPort,
	}

	postregsHost := os.Getenv("POSTGRES_HOST")
	if postregsHost == "" {
		postregsHost = "localhost"
	}

	postregsPortStr := os.Getenv("POSTGRES_PORT")
	if postregsPortStr == "" {
		postregsPortStr = "5432"
	}

	postregsPort, err := strconv.Atoi(postregsPortStr)
	if err != nil {
		return nil, fmt.Errorf("%s: invalid POSTGRES_PORT: %w", op, err)
	}

	postgresUser := os.Getenv("POSTGRES_USER")
	if postgresUser == "" {
		return nil, fmt.Errorf("%s: missing POSTGRES_USER", op)
	}

	postgresPassword := os.Getenv("POSTGRES_PASSWORD")
	if postgresPassword == "" {
		return nil, fmt.Errorf("%s: missing POSTGRES_PASSWORD", op)
	}

	postgresDB := os.Getenv("POSTGRES_DB")
	if postgresDB == "" {
		return nil, fmt.Errorf("%s: missing POSTGRES_DB", op)
	}

	postgresSSLMode := os.Getenv("POSTGRES_SSLMODE")
	if postgresSSLMode == "" {
		postgresSSLMode = "disable"
	}

	postgresCfg := PostgresConfig{
		User:     postgresUser,
		Password: postgresPassword,
		Name:     postgresDB,
		Host:     postregsHost,
		Port:     postregsPort,
		SSLMode:  postgresSSLMode,
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6380"
	}

	redisCfg := RedisConfig{
		Addr:     redisAddr,
		Password: "",
		DB:       0,
	}

	return &Config{
		Server:   serverCfg,
		Postgres: postgresCfg,
		Redis:    redisCfg,
	}, nil
}
