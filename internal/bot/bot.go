package bot

import (
	"context"
	"fmt"
	"log"

	"github.com/awwwm4n/price-tracker/internal/scraper"
	"github.com/awwwm4n/price-tracker/internal/storage"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot struct {
	api     *tgbotapi.BotAPI
	repo    storage.Repository
	scraper scraper.Scraper
}

// NewBot initializes a new Bot wrapper.
func NewBot(token string, repo storage.Repository, scraper scraper.Scraper) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("failed to create telegram bot: %w", err)
	}

	return &Bot{
		api:     api,
		repo:    repo,
		scraper: scraper,
	}, nil
}

// SetWebhook sets up the Telegram bot webhook endpoint.
func (b *Bot) SetWebhook(webhookURL string) error {
	webhookConfig, err := tgbotapi.NewWebhook(webhookURL)
	if err != nil {
		return fmt.Errorf("failed to build webhook config: %w", err)
	}

	_, err = b.api.Request(webhookConfig)
	if err != nil {
		return fmt.Errorf("failed to set webhook: %w", err)
	}

	log.Printf("Telegram webhook successfully set to: %s", webhookURL)
	return nil
}

// ProcessUpdate handles a single Telegram update payload.
func (b *Bot) ProcessUpdate(ctx context.Context, update tgbotapi.Update) error {
	if update.Message != nil {
		return b.handleMessage(ctx, update.Message)
	}
	if update.CallbackQuery != nil {
		return b.handleCallbackQuery(ctx, update.CallbackQuery)
	}
	return nil
}

// StartPolling starts long polling for updates (ideal for local testing).
func (b *Bot) StartPolling(ctx context.Context) {
	// Remove webhook first to enable polling
	_, _ = b.api.Request(tgbotapi.DeleteWebhookConfig{})

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)
	log.Println("Bot started polling for updates...")

	for {
		select {
		case <-ctx.Done():
			log.Println("Stopping polling due to context cancel...")
			return
		case update := <-updates:
			err := b.ProcessUpdate(ctx, update)
			if err != nil {
				log.Printf("Error processing update %d: %v", update.UpdateID, err)
			}
		}
	}
}

// SendMessage helper to push text messages.
func (b *Bot) SendMessage(chatID int64, text string) error {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdown
	msg.DisableWebPagePreview = true
	_, err := b.api.Send(msg)
	return err
}
