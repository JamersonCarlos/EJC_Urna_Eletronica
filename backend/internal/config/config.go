package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	AppEnv      string
	Port        string
	FrontendURL string
	JWTSecret   string
	JWTTTL      time.Duration

	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string

	CloudinaryURL    string
	CloudinaryFolder string
}

func Load() (*Config, error) {
	c := &Config{
		AppEnv:      env("APP_ENV", "development"),
		Port:        env("PORT", "8080"),
		FrontendURL: strings.TrimRight(env("FRONTEND_URL", "http://localhost:5173"), "/"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		JWTTTL:      12 * time.Hour,
		DBHost:      env("DB_HOST", "localhost"),
		DBPort:      env("DB_PORT", "3306"),
		DBUser:      env("DB_USER", "root"),
		DBPassword:  os.Getenv("DB_PASSWORD"),
		DBName:      env("DB_NAME", "urna"),

		CloudinaryURL:    os.Getenv("CLOUDINARY_URL"),
		CloudinaryFolder: env("CLOUDINARY_FOLDER", "urna/candidatos"),
	}
	if c.JWTSecret == "" {
		if c.AppEnv == "production" {
			return nil, fmt.Errorf("JWT_SECRET é obrigatório em produção")
		}
		c.JWTSecret = "dev-secret-change-me"
	}
	if len(c.JWTSecret) < 16 && c.AppEnv == "production" {
		return nil, fmt.Errorf("JWT_SECRET deve ter pelo menos 16 caracteres")
	}
	return c, nil
}

func (c *Config) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&loc=UTC&charset=utf8mb4&collation=utf8mb4_unicode_ci",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName)
}

// AllowedOrigins: FRONTEND_URL pode conter vários domínios separados por vírgula.
func (c *Config) AllowedOrigins() []string {
	var out []string
	for _, o := range strings.Split(c.FrontendURL, ",") {
		if o = strings.TrimSpace(strings.TrimRight(o, "/")); o != "" {
			out = append(out, o)
		}
	}
	return out
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
