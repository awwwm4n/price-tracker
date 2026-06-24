package main

import (
	"context"
	"log"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/awwwm4n/price-tracker/internal/bot"
	"github.com/awwwm4n/price-tracker/internal/config"
	"github.com/awwwm4n/price-tracker/internal/scheduler"
	"github.com/awwwm4n/price-tracker/internal/scraper"
	"github.com/awwwm4n/price-tracker/internal/storage"
)

var (
	priceScheduler *scheduler.PriceScheduler
)

func init() {
	ctx := context.TODO()
	cfg := config.LoadConfig()

	var awsOpts []func(*awsconfig.LoadOptions) error
	if cfg.DynamoDBEndpoint != "" {
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
	webScraper := scraper.NewHTTPScraper()

	// Initialize bot (required for scheduler to send alerts)
	botApp, err := bot.NewBot(cfg.TelegramBotToken, repo, webScraper)
	if err != nil {
		log.Fatalf("failed to initialize bot application: %v", err)
	}

	priceScheduler = scheduler.NewPriceScheduler(repo, webScraper, botApp)
}

func handleCron(ctx context.Context) error {
	log.Println("Cron Lambda triggered by EventBridge.")
	err := priceScheduler.RunChecks(ctx)
	if err != nil {
		log.Printf("ERROR: Price checks failed: %v", err)
		return err
	}
	return nil
}

func main() {
	lambda.Start(handleCron)
}
