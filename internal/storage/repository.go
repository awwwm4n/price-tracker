package storage

import (
	"context"
	"time"
)

// User represents a Telegram user.
type User struct {
	ChatID    int64     `dynamodbav:"ChatID"`
	Username  string    `dynamodbav:"Username"`
	FirstName string    `dynamodbav:"FirstName"`
	CreatedAt time.Time `dynamodbav:"CreatedAt"`
}

// Subscription represents a product price tracking subscription for a user.
type Subscription struct {
	ChatID        int64     `dynamodbav:"ChatID"`
	ASIN          string    `dynamodbav:"ASIN"`
	AmazonURL     string    `dynamodbav:"AmazonURL"`
	Title         string    `dynamodbav:"Title"`
	CurrentPrice  float64   `dynamodbav:"CurrentPrice"`
	TargetPrice   *float64  `dynamodbav:"TargetPrice,omitempty"` // pointer for optional field
	ImageURL      string    `dynamodbav:"ImageURL"`
	LastCheckedAt time.Time `dynamodbav:"LastCheckedAt"`
	CreatedAt     time.Time `dynamodbav:"CreatedAt"`
}

// Repository defines the storage operations.
type Repository interface {
	CreateOrUpdateUser(ctx context.Context, user *User) error
	GetUser(ctx context.Context, chatID int64) (*User, error)
	AddSubscription(ctx context.Context, sub *Subscription) error
	GetSubscription(ctx context.Context, chatID int64, asin string) (*Subscription, error)
	ListSubscriptionsByUser(ctx context.Context, chatID int64) ([]Subscription, error)
	RemoveSubscription(ctx context.Context, chatID int64, asin string) error
	ListAllSubscriptions(ctx context.Context) ([]Subscription, error)
	UpdateSubscriptionPrice(ctx context.Context, chatID int64, asin string, price float64, checkedAt time.Time) error
}
