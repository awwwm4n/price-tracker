package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/awwwm4n/price-tracker/internal/bot"
	"github.com/awwwm4n/price-tracker/internal/config"
	"github.com/awwwm4n/price-tracker/internal/scheduler"
	"github.com/awwwm4n/price-tracker/internal/scraper"
	"github.com/awwwm4n/price-tracker/internal/storage"
)

func main() {
	log.Println("Starting local price-tracker bot...")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Shutdown signal received. Cancelling context...")
		cancel()
	}()

	cfg := config.LoadConfig()
	if cfg.TelegramBotToken == "" {
		log.Fatal("ERROR: TELEGRAM_BOT_TOKEN environment variable is not set!")
	}

	var awsOpts []func(*awsconfig.LoadOptions) error
	if cfg.DynamoDBEndpoint != "" {
		log.Printf("Connecting to local DynamoDB endpoint: %s", cfg.DynamoDBEndpoint)
		awsOpts = append(awsOpts,
			awsconfig.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider("dummy", "dummy", "dummy"),
			),
		)
	} else {
		log.Println("Connecting to production AWS DynamoDB...")
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsOpts...)
	if err != nil {
		log.Fatalf("unable to load AWS SDK config: %v", err)
	}

	dbClient := dynamodb.NewFromConfig(awsCfg, func(o *dynamodb.Options) {
		if cfg.DynamoDBEndpoint != "" {
			o.BaseEndpoint = aws.String(cfg.DynamoDBEndpoint)
		}
	})

	if cfg.DynamoDBEndpoint != "" {
		log.Println("Checking and initializing local DynamoDB tables...")
		err = ensureTablesExist(ctx, dbClient, cfg.UsersTable, cfg.SubsTable)
		if err != nil {
			log.Fatalf("failed to initialize local database tables: %v", err)
		}
	}

	repo := storage.NewDynamoDBRepository(dbClient, cfg)
	webScraper := scraper.NewHTTPScraper(cfg)

	botApp, err := bot.NewBot(cfg.TelegramBotToken, repo, webScraper)
	if err != nil {
		log.Fatalf("failed to initialize bot application: %v", err)
	}

	// Run local cron ticker in background
	priceScheduler := scheduler.NewPriceScheduler(repo, webScraper, botApp)
	go func() {
		log.Println("Starting local cron scheduler background worker...")
		// Run initial check after 3 seconds so developer can see it run on startup
		time.Sleep(3 * time.Second)
		log.Println("Executing initial local price checks...")
		if err := priceScheduler.RunChecks(ctx); err != nil {
			log.Printf("ERROR running local price checks: %v", err)
		}

		ticker := time.NewTicker(3 * time.Minute) // Run every 3 minutes for local testing
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Println("Local scheduler background worker stopped.")
				return
			case <-ticker.C:
				log.Println("Executing scheduled hourly local price checks...")
				if err := priceScheduler.RunChecks(ctx); err != nil {
					log.Printf("ERROR running local price checks: %v", err)
				}
			}
		}
	}()

	// Run bot with Long Polling
	botApp.StartPolling(ctx)
}

func ensureTablesExist(ctx context.Context, client *dynamodb.Client, usersTable, subsTable string) error {
	tables, err := client.ListTables(ctx, &dynamodb.ListTablesInput{})
	if err != nil {
		return err
	}

	hasUsersTable := false
	hasSubsTable := false
	for _, name := range tables.TableNames {
		if name == usersTable {
			hasUsersTable = true
		}
		if name == subsTable {
			hasSubsTable = true
		}
	}

	if !hasUsersTable {
		log.Printf("Creating local table: %s", usersTable)
		_, err = client.CreateTable(ctx, &dynamodb.CreateTableInput{
			TableName: aws.String(usersTable),
			AttributeDefinitions: []types.AttributeDefinition{
				{AttributeName: aws.String("ChatID"), AttributeType: types.ScalarAttributeTypeN},
			},
			KeySchema: []types.KeySchemaElement{
				{AttributeName: aws.String("ChatID"), KeyType: types.KeyTypeHash},
			},
			ProvisionedThroughput: &types.ProvisionedThroughput{
				ReadCapacityUnits:  aws.Int64(5),
				WriteCapacityUnits: aws.Int64(5),
			},
		})
		if err != nil {
			return fmt.Errorf("failed to create users table: %w", err)
		}
	}

	if !hasSubsTable {
		log.Printf("Creating local table: %s", subsTable)
		_, err = client.CreateTable(ctx, &dynamodb.CreateTableInput{
			TableName: aws.String(subsTable),
			AttributeDefinitions: []types.AttributeDefinition{
				{AttributeName: aws.String("ChatID"), AttributeType: types.ScalarAttributeTypeN},
				{AttributeName: aws.String("ASIN"), AttributeType: types.ScalarAttributeTypeS},
			},
			KeySchema: []types.KeySchemaElement{
				{AttributeName: aws.String("ChatID"), KeyType: types.KeyTypeHash},
				{AttributeName: aws.String("ASIN"), KeyType: types.KeyTypeRange},
			},
			ProvisionedThroughput: &types.ProvisionedThroughput{
				ReadCapacityUnits:  aws.Int64(5),
				WriteCapacityUnits: aws.Int64(5),
			},
		})
		if err != nil {
			return fmt.Errorf("failed to create subscriptions table: %w", err)
		}
	}

	return nil
}
