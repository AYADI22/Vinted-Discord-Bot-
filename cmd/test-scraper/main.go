
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/2spy/vinted-discord-bot/internal/scrapers/vinted"
	"github.com/2spy/vinted-discord-bot/pkg/logger"
	"github.com/2spy/vinted-discord-bot/pkg/models"
	"github.com/go-redis/redis/v8"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {
	logger.Init()
	fmt.Println("🚀 Vinted Telegram Bot Starting...")

	// Config from environment
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatIDStr := os.Getenv("TELEGRAM_CHAT_ID")
	searchQuery := os.Getenv("SEARCH_QUERY")
	maxPriceStr := os.Getenv("MAX_PRICE")
	redisAddr := os.Getenv("REDIS_ADDR")
	rateLimitStr := os.Getenv("RATE_LIMIT_MS")

	if botToken == "" || chatIDStr == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID are required")
	}
	if searchQuery == "" {
		searchQuery = "iphone 13"
	}
	if maxPriceStr == "" {
		maxPriceStr = "500"
	}
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	if rateLimitStr == "" {
		rateLimitStr = "5000"
	}

	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		log.Fatalf("Invalid TELEGRAM_CHAT_ID: %v", err)
	}

	maxPrice, err := strconv.ParseFloat(maxPriceStr, 64)
	if err != nil {
		log.Fatalf("Invalid MAX_PRICE: %v", err)
	}

	rateLimit, err := strconv.Atoi(rateLimitStr)
	if err != nil {
		rateLimit = 5000
	}

	// Redis
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Redis connection failed: %v", err)
	}
	fmt.Println("✅ Redis connected")

	// Telegram
	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Fatalf("Telegram bot error: %v", err)
	}
	fmt.Printf("✅ Telegram bot: @%s\n", bot.Self.UserName)

	// Scraper
	scraper := vinted.NewVintedScraper()
	job := models.ScrapeJob{
		Query:    searchQuery,
		MaxPrice: maxPrice,
	}

	fmt.Printf("🔍 Monitoring: %s | Max price: %.0f€\n", searchQuery, maxPrice)

	for {
		items, err := scraper.Search(ctx, job)
		if err != nil {
			log.Printf("❌ Scrape error: %v", err)
			time.Sleep(time.Duration(rateLimit) * time.Millisecond)
			continue
		}

		newCount := 0
		for _, item := range items {
			if item.Price > maxPrice {
				continue
			}

			key := "vinted:seen:" + item.ID
			exists, err := rdb.Exists(ctx, key).Result()
			if err != nil || exists > 0 {
				continue
			}

			// Mark as seen (24h)
			rdb.Set(ctx, key, "1", 24*time.Hour)
			newCount++

			// Send Telegram message
			msg := fmt.Sprintf(
				"🛍 *%s*\n💰 %.2f %s\n🔗 [Voir sur Vinted](%s)",
				item.Title,
				item.Price,
				item.Currency,
				item.URL,
			)

			if item.ImageURL != "" {
				photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileURL(item.ImageURL))
				photo.Caption = msg
				photo.ParseMode = "Markdown"
				if _, err := bot.Send(photo); err != nil {
					// fallback to text
					textMsg := tgbotapi.NewMessage(chatID, msg)
					textMsg.ParseMode = "Markdown"
					bot.Send(textMsg)
				}
			} else {
				textMsg := tgbotapi.NewMessage(chatID, msg)
				textMsg.ParseMode = "Markdown"
				bot.Send(textMsg)
			}
		}

		if newCount > 0 {
			fmt.Printf("📨 Sent %d new items\n", newCount)
		}

		time.Sleep(time.Duration(rateLimit) * time.Millisecond)
	}
}
