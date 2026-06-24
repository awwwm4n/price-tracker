package config

import (
	"os"
)

// Config holds all the configuration variables parsed from the environment.
type Config struct {
	TelegramBotToken string
	UsersTable       string
	SubsTable        string
	AwsRegion        string
	Env              string
	DynamoDBEndpoint string // Used for local development/testing
	ScraperApiKey    string // Optional API key for ScraperAPI to bypass CAPTCHA on AWS Lambda
}

// LoadConfig loads the configuration from environment variables.
func LoadConfig() *Config {
	return &Config{
		TelegramBotToken: getEnv("TELEGRAM_BOT_TOKEN", ""),
		UsersTable:       getEnv("USERS_TABLE_NAME", "PriceTrackerUsers"),
		SubsTable:        getEnv("SUBS_TABLE_NAME", "PriceTrackerSubscriptions"),
		AwsRegion:        getEnv("AWS_REGION", "us-east-1"),
		Env:              getEnv("APP_ENV", "production"),
		DynamoDBEndpoint: getEnv("DYNAMODB_ENDPOINT", ""),
		ScraperApiKey:    getEnv("SCRAPER_API_KEY", ""),
	}
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}
