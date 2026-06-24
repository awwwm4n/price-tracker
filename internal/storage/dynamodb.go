package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/awwwm4n/price-tracker/internal/config"
)

type DynamoDBRepository struct {
	client     *dynamodb.Client
	usersTable string
	subsTable  string
}

// NewDynamoDBRepository creates a new DynamoDB Repository instance.
func NewDynamoDBRepository(client *dynamodb.Client, cfg *config.Config) *DynamoDBRepository {
	return &DynamoDBRepository{
		client:     client,
		usersTable: cfg.UsersTable,
		subsTable:  cfg.SubsTable,
	}
}

func (r *DynamoDBRepository) CreateOrUpdateUser(ctx context.Context, user *User) error {
	item, err := attributevalue.MarshalMap(user)
	if err != nil {
		return fmt.Errorf("failed to marshal user: %w", err)
	}

	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.usersTable),
		Item:      item,
	})
	if err != nil {
		return fmt.Errorf("failed to put user item: %w", err)
	}

	return nil
}

func (r *DynamoDBRepository) GetUser(ctx context.Context, chatID int64) (*User, error) {
	key := map[string]types.AttributeValue{
		"ChatID": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", chatID)},
	}

	res, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.usersTable),
		Key:       key,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get user item: %w", err)
	}

	if res.Item == nil {
		return nil, nil // Not found
	}

	var user User
	err = attributevalue.UnmarshalMap(res.Item, &user)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal user: %w", err)
	}

	return &user, nil
}

func (r *DynamoDBRepository) AddSubscription(ctx context.Context, sub *Subscription) error {
	item, err := attributevalue.MarshalMap(sub)
	if err != nil {
		return fmt.Errorf("failed to marshal subscription: %w", err)
	}

	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.subsTable),
		Item:      item,
	})
	if err != nil {
		return fmt.Errorf("failed to put subscription item: %w", err)
	}

	return nil
}

func (r *DynamoDBRepository) GetSubscription(ctx context.Context, chatID int64, asin string) (*Subscription, error) {
	key := map[string]types.AttributeValue{
		"ChatID": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", chatID)},
		"ASIN":   &types.AttributeValueMemberS{Value: asin},
	}

	res, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.subsTable),
		Key:       key,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}

	if res.Item == nil {
		return nil, nil // Not found
	}

	var sub Subscription
	err = attributevalue.UnmarshalMap(res.Item, &sub)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal subscription: %w", err)
	}

	return &sub, nil
}

func (r *DynamoDBRepository) ListSubscriptionsByUser(ctx context.Context, chatID int64) ([]Subscription, error) {
	keyCond := expression.Key("ChatID").Equal(expression.Value(chatID))
	expr, err := expression.NewBuilder().WithKeyCondition(keyCond).Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build expression: %w", err)
	}

	input := &dynamodb.QueryInput{
		TableName:                 aws.String(r.subsTable),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	}

	res, err := r.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query subscriptions: %w", err)
	}

	var subs []Subscription
	err = attributevalue.UnmarshalListOfMaps(res.Items, &subs)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal subscriptions list: %w", err)
	}

	return subs, nil
}

func (r *DynamoDBRepository) RemoveSubscription(ctx context.Context, chatID int64, asin string) error {
	key := map[string]types.AttributeValue{
		"ChatID": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", chatID)},
		"ASIN":   &types.AttributeValueMemberS{Value: asin},
	}

	_, err := r.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(r.subsTable),
		Key:       key,
	})
	if err != nil {
		return fmt.Errorf("failed to delete subscription: %w", err)
	}

	return nil
}

func (r *DynamoDBRepository) ListAllSubscriptions(ctx context.Context) ([]Subscription, error) {
	var subs []Subscription
	var lastEvaluatedKey map[string]types.AttributeValue

	for {
		input := &dynamodb.ScanInput{
			TableName:         aws.String(r.subsTable),
			ExclusiveStartKey: lastEvaluatedKey,
		}

		res, err := r.client.Scan(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to scan subscriptions: %w", err)
		}

		var page []Subscription
		err = attributevalue.UnmarshalListOfMaps(res.Items, &page)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal scanned subscriptions: %w", err)
		}

		subs = append(subs, page...)
		lastEvaluatedKey = res.LastEvaluatedKey

		if lastEvaluatedKey == nil {
			break
		}
	}

	return subs, nil
}

func (r *DynamoDBRepository) UpdateSubscriptionPrice(ctx context.Context, chatID int64, asin string, price float64, checkedAt time.Time) error {
	key := map[string]types.AttributeValue{
		"ChatID": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", chatID)},
		"ASIN":   &types.AttributeValueMemberS{Value: asin},
	}

	update := expression.Set(
		expression.Name("CurrentPrice"), expression.Value(price),
	).Set(
		expression.Name("LastCheckedAt"), expression.Value(checkedAt.Format(time.RFC3339)),
	)


	// We verify that the item exists before updating. Optional, but prevents creating new items with incomplete info.
	condition := expression.AttributeExists(expression.Name("ChatID")).And(expression.AttributeExists(expression.Name("ASIN")))
	exprWithCond, err := expression.NewBuilder().WithUpdate(update).WithCondition(condition).Build()
	if err != nil {
		return fmt.Errorf("failed to build expression with condition: %w", err)
	}

	_, err = r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 aws.String(r.subsTable),
		Key:                       key,
		UpdateExpression:          exprWithCond.Update(),
		ConditionExpression:       exprWithCond.Condition(),
		ExpressionAttributeNames:  exprWithCond.Names(),
		ExpressionAttributeValues: exprWithCond.Values(),
	})
	if err != nil {
		var condFailedErr *types.ConditionalCheckFailedException
		if errors.As(err, &condFailedErr) {
			return fmt.Errorf("subscription not found for updating price: %w", err)
		}
		return fmt.Errorf("failed to update subscription price: %w", err)
	}

	return nil
}
