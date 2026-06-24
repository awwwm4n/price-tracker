package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/awwwm4n/price-tracker/internal/bot"
	"github.com/awwwm4n/price-tracker/internal/scraper"
	"github.com/awwwm4n/price-tracker/internal/storage"
)

type PriceScheduler struct {
	repo    storage.Repository
	scraper scraper.Scraper
	bot     *bot.Bot
}

func NewPriceScheduler(repo storage.Repository, scraper scraper.Scraper, bot *bot.Bot) *PriceScheduler {
	return &PriceScheduler{
		repo:    repo,
		scraper: scraper,
		bot:     bot,
	}
}

// RunChecks performs the hourly price check across all user subscriptions.
func (s *PriceScheduler) RunChecks(ctx context.Context) error {
	log.Println("Starting scheduled price checks...")

	// 1. Fetch all active subscriptions from database
	subs, err := s.repo.ListAllSubscriptions(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch all subscriptions: %w", err)
	}

	if len(subs) == 0 {
		log.Println("No active subscriptions found in the database. Ending checks.")
		return nil
	}

	// 2. Group subscriptions by ASIN to avoid scraping the same URL multiple times
	asinGroups := make(map[string][]storage.Subscription)
	for _, sub := range subs {
		asinGroups[sub.ASIN] = append(asinGroups[sub.ASIN], sub)
	}

	log.Printf("Found %d unique products to scrape from %d total subscriptions.", len(asinGroups), len(subs))

	// 3. Process each group
	for asin, group := range asinGroups {
		// Use the URL from the first subscription in the group
		sampleSub := group[0]
		log.Printf("Scraping product ASIN: %s, URL: %s", asin, sampleSub.AmazonURL)

		product, err := s.scraper.Scrape(ctx, sampleSub.AmazonURL)
		if err != nil {
			log.Printf("ERROR: Failed to scrape ASIN %s: %v", asin, err)
			continue
		}

		log.Printf("Successfully scraped ASIN %s. Title: %s. Price: %.2f", asin, product.Title, product.Price)

		// 4. Update subscriptions and notify users of price drops
		for _, sub := range group {
			oldPrice := sub.CurrentPrice
			newPrice := product.Price

			// Check for price drop
			if newPrice < oldPrice {
				savings := oldPrice - newPrice
				savingsPercent := (savings / oldPrice) * 100

				alertText := fmt.Sprintf(
					"📉 *Price Drop Alert! Price has decreased!*\n\n📦 *Product:* %s\n🔥 *New Price:* %.2f\n💰 *Previous Price:* %.2f (Saved %.2f - %.1f%%!)\n\n🔗 [Buy on Amazon](%s)",
					sub.Title,
					newPrice,
					oldPrice,
					savings,
					savingsPercent,
					sub.AmazonURL,
				)

				log.Printf("Price drop detected for Chat %d, ASIN %s: %.2f -> %.2f. Sending alert.", sub.ChatID, asin, oldPrice, newPrice)

				// Notify user
				err := s.bot.SendMessage(sub.ChatID, alertText)
				if err != nil {
					log.Printf("ERROR: Failed to send price drop notification to Chat %d: %v", sub.ChatID, err)
				}
			}

			// 5. Update subscription record with the latest price and checked timestamp
			err = s.repo.UpdateSubscriptionPrice(ctx, sub.ChatID, asin, newPrice, time.Now())
			if err != nil {
				log.Printf("ERROR: Failed to update price in database for Chat %d, ASIN %s: %v", sub.ChatID, asin, err)
			}
		}

		// Be nice to Amazon's servers. Wait a short interval between scrapes
		select {
		case <-ctx.Done():
			log.Println("Price checks cancelled early by context.")
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}

	log.Println("Price checks completed successfully.")
	return nil
}
