package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/awwwm4n/price-tracker/internal/bot"
	"github.com/awwwm4n/price-tracker/internal/config"
	"github.com/awwwm4n/price-tracker/internal/scraper"
	"github.com/awwwm4n/price-tracker/internal/storage"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var (
	botApp *bot.Bot
)

func init() {
	// Setup configurations during Lambda container initialization to optimize cold starts
	ctx := context.TODO()
	cfg := config.LoadConfig()

	var awsOpts []func(*awsconfig.LoadOptions) error
	if cfg.DynamoDBEndpoint != "" {
		// Allows using DynamoDB local in testing or local runners
		awsOpts = append(awsOpts,
			awsconfig.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider("dummy", "dummy", "dummy"),
			),
		)
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsOpts...)
	if err != nil {
		log.Fatalf("unable to load SDK config: %v", err)
	}

	dbClient := dynamodb.NewFromConfig(awsCfg, func(o *dynamodb.Options) {
		if cfg.DynamoDBEndpoint != "" {
			o.BaseEndpoint = aws.String(cfg.DynamoDBEndpoint)
		}
	})
	repo := storage.NewDynamoDBRepository(dbClient, cfg)
	webScraper := scraper.NewHTTPScraper(cfg)

	botApp, err = bot.NewBot(cfg.TelegramBotToken, repo, webScraper)
	if err != nil {
		log.Fatalf("failed to initialize bot application: %v", err)
	}
}

func handleRequest(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	log.Printf("Received Telegram Webhook Update: %s", request.Body)

	var update tgbotapi.Update
	err := json.Unmarshal([]byte(request.Body), &update)
	if err != nil {
		log.Printf("ERROR: Failed to unmarshal update JSON: %v", err)
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusBadRequest,
			Body:       "Invalid JSON payload",
		}, nil
	}

	err = botApp.ProcessUpdate(ctx, update)
	if err != nil {
		log.Printf("ERROR: Failed to process update: %v", err)
		// We return a 200 OK anyway so Telegram doesn't retry indefinitely
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusOK,
			Body:       "Error during processing, logged.",
		}, nil
	}

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Body:       "OK",
	}, nil
}

func main() {
	lambda.Start(handleRequest)
}
