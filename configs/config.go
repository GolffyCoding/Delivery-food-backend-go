package configs

import (
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Redis    RedisConfig
	JWT      JWTConfig
	NATS     NATSConfig
}

type ServerConfig struct {
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

type DatabaseConfig struct {
	Host            string
	Port            string
	User            string
	Password        string
	DBName          string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type RedisConfig struct {
	Host     string
	Port     string
	Password string
	DB       int
}

type JWTConfig struct {
	AccessTokenSecret  string
	RefreshTokenSecret string
	AccessTokenTTL     time.Duration
	RefreshTokenTTL    time.Duration
	Issuer             string
}

type NATSConfig struct {
	URL     string
	Enabled bool
}

func Load() (*Config, error) {
	viper.SetConfigName(".env")
	viper.SetConfigType("env")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./configs")
	viper.AutomaticEnv()

	setDefaults()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
	}

	cfg := &Config{
		Server: ServerConfig{
			Port:         viper.GetString("SERVER_PORT"),
			ReadTimeout:  viper.GetDuration("SERVER_READ_TIMEOUT"),
			WriteTimeout: viper.GetDuration("SERVER_WRITE_TIMEOUT"),
			IdleTimeout:  viper.GetDuration("SERVER_IDLE_TIMEOUT"),
		},
		Database: DatabaseConfig{
			Host:            viper.GetString("DB_HOST"),
			Port:            viper.GetString("DB_PORT"),
			User:            viper.GetString("DB_USER"),
			Password:        viper.GetString("DB_PASSWORD"),
			DBName:          viper.GetString("DB_NAME"),
			SSLMode:         viper.GetString("DB_SSL_MODE"),
			MaxOpenConns:    viper.GetInt("DB_MAX_OPEN_CONNS"),
			MaxIdleConns:    viper.GetInt("DB_MAX_IDLE_CONNS"),
			ConnMaxLifetime: viper.GetDuration("DB_CONN_MAX_LIFETIME"),
		},
		Redis: RedisConfig{
			Host:     viper.GetString("REDIS_HOST"),
			Port:     viper.GetString("REDIS_PORT"),
			Password: viper.GetString("REDIS_PASSWORD"),
			DB:       viper.GetInt("REDIS_DB"),
		},
		JWT: JWTConfig{
			AccessTokenSecret:  viper.GetString("JWT_ACCESS_SECRET"),
			RefreshTokenSecret: viper.GetString("JWT_REFRESH_SECRET"),
			AccessTokenTTL:     viper.GetDuration("JWT_ACCESS_TTL"),
			RefreshTokenTTL:    viper.GetDuration("JWT_REFRESH_TTL"),
			Issuer:             viper.GetString("JWT_ISSUER"),
		},
		NATS: NATSConfig{
			URL:     viper.GetString("NATS_URL"),
			Enabled: viper.GetBool("NATS_ENABLED"),
		},
	}

	return cfg, nil
}

func setDefaults() {
	viper.SetDefault("SERVER_PORT", ":8080")
	viper.SetDefault("SERVER_READ_TIMEOUT", 15*time.Second)
	viper.SetDefault("SERVER_WRITE_TIMEOUT", 15*time.Second)
	viper.SetDefault("SERVER_IDLE_TIMEOUT", 60*time.Second)

	viper.SetDefault("DB_HOST", "localhost")
	viper.SetDefault("DB_PORT", "5432")
	viper.SetDefault("DB_USER", "opendelivery")
	viper.SetDefault("DB_PASSWORD", "opendelivery")
	viper.SetDefault("DB_NAME", "opendelivery")
	viper.SetDefault("DB_SSL_MODE", "disable")
	viper.SetDefault("DB_MAX_OPEN_CONNS", 25)
	viper.SetDefault("DB_MAX_IDLE_CONNS", 10)
	viper.SetDefault("DB_CONN_MAX_LIFETIME", 5*time.Minute)

	viper.SetDefault("REDIS_HOST", "localhost")
	viper.SetDefault("REDIS_PORT", "6379")
	viper.SetDefault("REDIS_PASSWORD", "")
	viper.SetDefault("REDIS_DB", 0)

	viper.SetDefault("JWT_ACCESS_SECRET", "change-me-in-production-access")
	viper.SetDefault("JWT_REFRESH_SECRET", "change-me-in-production-refresh")
	viper.SetDefault("JWT_ACCESS_TTL", 15*time.Minute)
	viper.SetDefault("JWT_REFRESH_TTL", 168*time.Hour)
	viper.SetDefault("JWT_ISSUER", "opendelivery")

	viper.SetDefault("NATS_URL", "nats://localhost:4222")
	viper.SetDefault("NATS_ENABLED", false)
}
