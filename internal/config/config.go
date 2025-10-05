package config

import (
	"os"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

type Config struct {
	Port         string
	StoragePath  string
	APIKey       string
	RequireAPIKey bool
	S3           bool
	S3Endpoint   string
	S3AccessKey  string
	S3SecretKey  string
	S3Region     string
	S3Bucket     string
	S3Client     *s3.S3
}

func Load() *Config {
	cfg := &Config{
		Port:         getEnv("PORT", "3000"),
		StoragePath:  getEnv("STORAGE_PATH", "./storage"),
		APIKey:       getEnv("API_KEY", "super-secret-key"),
		RequireAPIKey: getEnvBool("REQUIRE_API_KEY", true),
		S3:           getEnvBool("S3", false),
		S3Endpoint:   getEnv("S3_ENDPOINT", ""),
		S3AccessKey:  getEnv("S3_ACCESS_KEY", ""),
		S3SecretKey:  getEnv("S3_SECRET_KEY", ""),
		S3Region:     getEnv("S3_REGION", "us-east-1"),
		S3Bucket:     getEnv("S3_BUCKET", "viespirkiai"),
	}

	// Initialize S3 client if S3 is enabled
	if cfg.S3 {
		sess, err := session.NewSession(&aws.Config{
			Endpoint:    aws.String(cfg.S3Endpoint),
			Region:      aws.String(cfg.S3Region),
			Credentials: credentials.NewStaticCredentials(cfg.S3AccessKey, cfg.S3SecretKey, ""),
		})
		if err != nil {
			panic("Failed to create S3 session: " + err.Error())
		}
		cfg.S3Client = s3.New(sess)
	}

	return cfg
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}
