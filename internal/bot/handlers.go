package bot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/awwwm4n/price-tracker/internal/scraper"
	"github.com/awwwm4n/price-tracker/internal/storage"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	welcomeMessage = `👋 *Welcome to the Amazon Price Tracker Bot!*

I can help you monitor Amazon product prices and notify you the second they drop!

*Commands:*
/list - 📋 Show all products you are currently tracking.
/help - ℹ️ View instructions.

*How to track a product:*
Simply paste any Amazon product link directly into this chat! I'll extract the details, check the current price, and start tracking it hourly for you. Try it now!`
)

func (b *Bot) handleMessage(ctx context.Context, msg *tgbotapi.Message) error {
	// 1. Ensure user exists in database
	user := &storage.User{
		ChatID:    msg.Chat.ID,
		Username:  msg.From.UserName,
		FirstName: msg.From.FirstName,
		CreatedAt: time.Now(),
	}
	err := b.repo.CreateOrUpdateUser(ctx, user)
	if err != nil {
		log.Printf("Failed to record user %d in DB: %v", msg.Chat.ID, err)
	}

	// 2. Handle commands
	if msg.IsCommand() {
		switch msg.Command() {
		case "start":
			return b.SendMessage(msg.Chat.ID, welcomeMessage)
		case "help":
			return b.SendMessage(msg.Chat.ID, welcomeMessage)
		case "list":
			return b.handleListCommand(ctx, msg.Chat.ID)
		default:
			return b.SendMessage(msg.Chat.ID, "❌ Unknown command. Type /help to see what I can do.")
		}
	}

	// 3. Handle free text (Check if it's an Amazon URL)
	text := strings.TrimSpace(msg.Text)
	if strings.Contains(text, "amazon.") || strings.Contains(text, "amzn.") {
		return b.handleProductRegistration(ctx, msg.Chat.ID, text)
	}

	// Otherwise, fallback response
	return b.SendMessage(msg.Chat.ID, "💡 I didn't recognize that command. To track a product, just paste its Amazon link here! For example:\n`https://www.amazon.in/dp/B0CXM1K4TL`")
}

func (b *Bot) handleListCommand(ctx context.Context, chatID int64) error {
	subs, err := b.repo.ListSubscriptionsByUser(ctx, chatID)
	if err != nil {
		return b.SendMessage(chatID, "❌ Failed to fetch your tracked items. Please try again later.")
	}

	if len(subs) == 0 {
		return b.SendMessage(chatID, "📋 *You are not tracking any products yet.*\n\nPaste an Amazon product link to start tracking it!")
	}

	err = b.SendMessage(chatID, fmt.Sprintf("📋 *You are tracking %d product(s):*", len(subs)))
	if err != nil {
		return err
	}

	// Send each tracked product as a separate card/message
	for _, sub := range subs {
		caption := fmt.Sprintf(
			"📦 *%s*\n💰 *Current Price:* %.2f\n📝 *ASIN:* `%s`",
			truncateString(sub.Title, 100),
			sub.CurrentPrice,
			sub.ASIN,
		)

		// Inline button to delete
		inlineKeyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("❌ Stop Tracking", fmt.Sprintf("remove:%s", sub.ASIN)),
			),
		)

		if sub.ImageURL != "" {
			photoMsg := tgbotapi.NewPhoto(chatID, tgbotapi.FileURL(sub.ImageURL))
			photoMsg.Caption = caption
			photoMsg.ParseMode = tgbotapi.ModeMarkdown
			photoMsg.ReplyMarkup = inlineKeyboard
			_, err = b.api.Send(photoMsg)
		} else {
			textMsg := tgbotapi.NewMessage(chatID, caption+"\n🔗 [Link to Product]("+sub.AmazonURL+")")
			textMsg.ParseMode = tgbotapi.ModeMarkdown
			textMsg.ReplyMarkup = inlineKeyboard
			textMsg.DisableWebPagePreview = true
			_, err = b.api.Send(textMsg)
		}

		if err != nil {
			log.Printf("Failed to send subscription card for ASIN %s: %v", sub.ASIN, err)
		}
	}

	return nil
}

func (b *Bot) handleProductRegistration(ctx context.Context, chatID int64, rawURL string) error {
	asin, normalizedURL, err := scraper.ParseAmazonURL(rawURL)
	if err != nil {
		return b.SendMessage(chatID, fmt.Sprintf("❌ *Invalid URL:* %v\nPlease make sure you are pasting a valid Amazon product link.", err))
	}

	// Send loading message
	loadingMsg := tgbotapi.NewMessage(chatID, "🔍 *Fetching product details from Amazon...* Please wait.")
	loadingMsg.ParseMode = tgbotapi.ModeMarkdown
	sentLoading, err := b.api.Send(loadingMsg)
	if err != nil {
		return err
	}

	// Helper to delete loading message and send response
	deleteLoading := func() {
		deleteMsg := tgbotapi.NewDeleteMessage(chatID, sentLoading.MessageID)
		_, _ = b.api.Request(deleteMsg)
	}

	// Scrape product
	product, err := b.scraper.Scrape(ctx, normalizedURL)
	if err != nil {
		deleteLoading()
		log.Printf("Failed scraping product for URL %s: %v", normalizedURL, err)
		return b.SendMessage(chatID, "❌ *Failed to fetch product details.*\n\nAmazon might be rate-limiting us or checking for bot activity. Please try again in a few minutes.")
	}

	// Check if user is already tracking this product
	existing, err := b.repo.GetSubscription(ctx, chatID, asin)
	if err == nil && existing != nil {
		deleteLoading()
		return b.SendMessage(chatID, fmt.Sprintf("ℹ️ You are already tracking *%s*.\n💰 Current Price: %.2f", existing.Title, existing.CurrentPrice))
	}

	// Create subscription
	sub := &storage.Subscription{
		ChatID:        chatID,
		ASIN:          asin,
		AmazonURL:     normalizedURL,
		Title:         product.Title,
		CurrentPrice:  product.Price,
		ImageURL:      product.ImageURL,
		LastCheckedAt: time.Now(),
		CreatedAt:     time.Now(),
	}

	err = b.repo.AddSubscription(ctx, sub)
	if err != nil {
		deleteLoading()
		log.Printf("Failed to save subscription: %v", err)
		return b.SendMessage(chatID, "❌ Failed to save tracking details. Please try again.")
	}

	deleteLoading()

	// Notify user of successful tracking
	successMessage := fmt.Sprintf(
		"🚀 *Now tracking price changes!*\n\n📦 *Product:* %s\n💰 *Initial Price:* %.2f\n\nI will check this product hourly and message you if the price drops below %.2f!",
		product.Title,
		product.Price,
		product.Price,
	)

	if product.ImageURL != "" {
		photoMsg := tgbotapi.NewPhoto(chatID, tgbotapi.FileURL(product.ImageURL))
		photoMsg.Caption = successMessage
		photoMsg.ParseMode = tgbotapi.ModeMarkdown
		_, err = b.api.Send(photoMsg)
		return err
	}

	return b.SendMessage(chatID, successMessage)
}

func (b *Bot) handleCallbackQuery(ctx context.Context, query *tgbotapi.CallbackQuery) error {
	data := query.Data

	// Answer callback query so Telegram spinner stops loading
	callbackResponse := tgbotapi.NewCallback(query.ID, "")
	defer func() {
		_, _ = b.api.Request(callbackResponse)
	}()

	if strings.HasPrefix(data, "remove:") {
		asin := strings.TrimPrefix(data, "remove:")
		chatID := query.Message.Chat.ID

		err := b.repo.RemoveSubscription(ctx, chatID, asin)
		if err != nil {
			callbackResponse.Text = "❌ Failed to stop tracking. Try again."
			return err
		}

		callbackResponse.Text = "✅ Stopped tracking product!"

		// Update card message to reflect deletion
		var editMsg tgbotapi.EditMessageTextConfig
		if query.Message.Photo != nil {
			// If it was a photo message, we can't edit the caption to text directly without deleting photo,
			// but we can edit caption of the photo message!
			editCaption := tgbotapi.NewEditMessageCaption(chatID, query.Message.MessageID, fmt.Sprintf("❌ *Stopped tracking:*\nASIN: `%s`", asin))
			editCaption.ParseMode = tgbotapi.ModeMarkdown
			_, err = b.api.Send(editCaption)
		} else {
			// Plain text message
			editMsg = tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, fmt.Sprintf("❌ *Stopped tracking:* ASIN `%s`", asin))
			editMsg.ParseMode = tgbotapi.ModeMarkdown
			_, err = b.api.Send(editMsg)
		}
		return err
	}

	return nil
}

func truncateString(str string, length int) string {
	if len(str) > length {
		return str[0:length] + "..."
	}
	return str
}
