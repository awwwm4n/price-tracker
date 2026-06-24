package storage

import (
	"context"
	"log"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/awwwm4n/price-tracker/internal/config"
)

func TestDynamoDBIntegration(t *testing.T) {
	// Set up local config pointing to DynamoDB Local
	cfg := &config.Config{
		UsersTable:       "TestPriceTrackerUsers",
		SubsTable:        "TestPriceTrackerSubscriptions",
		AwsRegion:        "us-east-1",
		DynamoDBEndpoint: "http://127.0.0.1:8000",
	}

	// Verify if local DynamoDB is running, skip if not reachable (e.g. clean CI environments)
	connCheck := &http.Client{Timeout: 500 * time.Millisecond}
	_, err := connCheck.Get(cfg.DynamoDBEndpoint)
	if err != nil {
		t.Skip("Skipping integration test: local DynamoDB instance is not reachable")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Load AWS config with local endpoint and static credentials
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.AwsRegion),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("dummy", "dummy", "dummy"),
		),
	)
	if err != nil {
		t.Fatalf("failed to load AWS config: %v", err)
	}

	client := dynamodb.NewFromConfig(awsCfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String(cfg.DynamoDBEndpoint)
	})

	// Clean up tables first if they exist from previous runs
	deleteTableIfExists(ctx, client, cfg.UsersTable)
	deleteTableIfExists(ctx, client, cfg.SubsTable)

	// Create test tables
	err = createTestTables(ctx, client, cfg.UsersTable, cfg.SubsTable)
	if err != nil {
		t.Fatalf("failed to create tables: %v", err)
	}
	defer func() {
		// Clean up after test finishes
		deleteTableIfExists(ctx, client, cfg.UsersTable)
		deleteTableIfExists(ctx, client, cfg.SubsTable)
	}()

	repo := NewDynamoDBRepository(client, cfg)

	// --- 1. Test CreateOrUpdateUser & GetUser ---
	testUser := &User{
		ChatID:    123456789,
		Username:  "test_user",
		FirstName: "Test",
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}

	err = repo.CreateOrUpdateUser(ctx, testUser)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	fetchedUser, err := repo.GetUser(ctx, testUser.ChatID)
	if err != nil {
		t.Fatalf("failed to get user: %v", err)
	}
	if fetchedUser == nil {
		t.Fatal("expected user to be found, got nil")
	}
	if fetchedUser.Username != testUser.Username || fetchedUser.FirstName != testUser.FirstName {
		t.Errorf("fetched user %+v does not match expected %+v", fetchedUser, testUser)
	}

	// --- 2. Test AddSubscription & GetSubscription ---
	targetPrice := 499.99
	testSub := &Subscription{
		ChatID:        testUser.ChatID,
		ASIN:          "B0CXM1K4TL",
		AmazonURL:     "https://www.amazon.in/dp/B0CXM1K4TL",
		Title:         "Sample Amazon Product",
		CurrentPrice:  599.99,
		TargetPrice:   &targetPrice,
		ImageURL:      "https://images.amazon.com/sample.jpg",
		LastCheckedAt: time.Now().UTC().Truncate(time.Second),
		CreatedAt:     time.Now().UTC().Truncate(time.Second),
	}

	err = repo.AddSubscription(ctx, testSub)
	if err != nil {
		t.Fatalf("failed to add subscription: %v", err)
	}

	fetchedSub, err := repo.GetSubscription(ctx, testSub.ChatID, testSub.ASIN)
	if err != nil {
		t.Fatalf("failed to get subscription: %v", err)
	}
	if fetchedSub == nil {
		t.Fatal("expected subscription to be found, got nil")
	}
	if fetchedSub.Title != testSub.Title || fetchedSub.CurrentPrice != testSub.CurrentPrice {
		t.Errorf("fetched sub %+v does not match expected %+v", fetchedSub, testSub)
	}

	// --- 3. Test ListSubscriptionsByUser ---
	subs, err := repo.ListSubscriptionsByUser(ctx, testUser.ChatID)
	if err != nil {
		t.Fatalf("failed to list subscriptions: %v", err)
	}
	if len(subs) != 1 {
		t.Errorf("expected 1 subscription, got %d", len(subs))
	}

	// --- 4. Test UpdateSubscriptionPrice ---
	newPrice := 549.99
	checkedAt := time.Now().UTC().Truncate(time.Second)
	err = repo.UpdateSubscriptionPrice(ctx, testSub.ChatID, testSub.ASIN, newPrice, checkedAt)
	if err != nil {
		t.Fatalf("failed to update price: %v", err)
	}

	updatedSub, err := repo.GetSubscription(ctx, testSub.ChatID, testSub.ASIN)
	if err != nil {
		t.Fatalf("failed to get updated sub: %v", err)
	}
	if updatedSub.CurrentPrice != newPrice {
		t.Errorf("expected price to be updated to %f, got %f", newPrice, updatedSub.CurrentPrice)
	}

	// --- 5. Test ListAllSubscriptions ---
	allSubs, err := repo.ListAllSubscriptions(ctx)
	if err != nil {
		t.Fatalf("failed to list all subscriptions: %v", err)
	}
	if len(allSubs) != 1 {
		t.Errorf("expected 1 total subscription in DB, got %d", len(allSubs))
	}

	// --- 6. Test RemoveSubscription ---
	err = repo.RemoveSubscription(ctx, testSub.ChatID, testSub.ASIN)
	if err != nil {
		t.Fatalf("failed to remove subscription: %v", err)
	}

	deletedSub, err := repo.GetSubscription(ctx, testSub.ChatID, testSub.ASIN)
	if err != nil {
		t.Fatalf("failed to check deleted subscription: %v", err)
	}
	if deletedSub != nil {
		t.Fatal("expected subscription to be deleted, but it was found")
	}
}

func deleteTableIfExists(ctx context.Context, client *dynamodb.Client, tableName string) {
	_, err := client.DeleteTable(ctx, &dynamodb.DeleteTableInput{
		TableName: aws.String(tableName),
	})
	if err != nil {
		// Log but ignore error since table might not exist
		log.Printf("Table deletion skipped for %s (probably doesn't exist)", tableName)
	}
}

func createTestTables(ctx context.Context, client *dynamodb.Client, usersTable, subsTable string) error {
	_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{
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
		return err
	}

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
	return err
}
