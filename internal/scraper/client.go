package scraper

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2.1 Safari/605.1.15",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
}

type HTTPScraper struct {
	client *http.Client
}

func NewHTTPScraper() *HTTPScraper {
	return &HTTPScraper{
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (s *HTTPScraper) Scrape(ctx context.Context, rawURL string) (*Product, error) {
	asin, normalizedURL, err := ParseAmazonURL(rawURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Amazon URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", normalizedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set realistic headers
	req.Header.Set("User-Agent", getRandomUserAgent())
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Ch-Ua", `"Not_A Brand";v="8", "Chromium";v="120", "Google Chrome";v="120"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed HTTP request to Amazon: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 503 {
		return nil, fmt.Errorf("amazon rate limited (503 Service Unavailable / Captcha challenge)")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("received non-200 status code: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML body: %w", err)
	}

	// 1. Extract Title
	title := strings.TrimSpace(doc.Find("#productTitle").First().Text())
	if title == "" {
		// Fallback checking for CAPTCHA/Robot detection page
		if doc.Find("form[action='/errors/validateCaptcha']").Length() > 0 {
			return nil, fmt.Errorf("amazon returned captcha challenge page")
		}
		return nil, fmt.Errorf("could not find product title (likely blocked/different layout)")
	}

	// 2. Extract Price
	price, err := extractPrice(doc)
	if err != nil {
		return nil, fmt.Errorf("failed to extract product price: %w", err)
	}

	// 3. Extract Image URL
	imageURL := extractImage(doc)

	return &Product{
		ASIN:     asin,
		Title:    title,
		Price:    price,
		ImageURL: imageURL,
		URL:      normalizedURL,
	}, nil
}

func getRandomUserAgent() string {
	// Seed random using nano timestamp to ensure variance
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return userAgents[r.Intn(len(userAgents))]
}

func extractPrice(doc *goquery.Document) (float64, error) {
	// Ordered selectors to find price
	priceSelectors := []string{
		"span.priceToPay span.a-offscreen",
		"span.apexPriceToPay span.a-offscreen",
		"#price_inside_buybox",
		"span.a-price span.a-offscreen",
		"#priceblock_ourprice",
		"#priceblock_dealprice",
	}

	for _, selector := range priceSelectors {
		rawPrice := strings.TrimSpace(doc.Find(selector).First().Text())
		if rawPrice != "" {
			price, err := parsePriceString(rawPrice)
			if err == nil && price > 0 {
				return price, nil
			}
		}
	}

	// Fallback to searching individual whole/fraction tags (e.g. inside a-price)
	whole := strings.TrimSpace(doc.Find("span.a-price-whole").First().Text())
	fraction := strings.TrimSpace(doc.Find("span.a-price-fraction").First().Text())
	if whole != "" {
		if fraction == "" {
			fraction = "00"
		}
		// e.g. "99." or "99" and "99"
		whole = strings.TrimSuffix(whole, ".")
		whole = strings.TrimSuffix(whole, ",")
		rawPrice := fmt.Sprintf("%s.%s", whole, fraction)
		price, err := parsePriceString(rawPrice)
		if err == nil && price > 0 {
			return price, nil
		}
	}

	return 0, fmt.Errorf("could not locate valid price element in HTML")
}

func parsePriceString(rawPrice string) (float64, error) {
	rawPrice = strings.TrimSpace(rawPrice)
	var cleaned strings.Builder
	for _, char := range rawPrice {
		if (char >= '0' && char <= '9') || char == '.' || char == ',' {
			cleaned.WriteRune(char)
		}
	}
	s := cleaned.String()
	if s == "" {
		return 0, fmt.Errorf("no digits found in price string")
	}

	// Heuristics for thousands/decimal comma vs dot
	if strings.Contains(s, ",") && strings.Contains(s, ".") {
		lastDot := strings.LastIndex(s, ".")
		lastComma := strings.LastIndex(s, ",")
		if lastDot > lastComma {
			// Dot is decimal separator, comma is thousands separator
			s = strings.ReplaceAll(s, ",", "")
		} else {
			// Comma is decimal separator, dot is thousands separator
			s = strings.ReplaceAll(s, ".", "")
			s = strings.ReplaceAll(s, ",", ".")
		}
	} else if strings.Contains(s, ",") {
		parts := strings.Split(s, ",")
		if len(parts[len(parts)-1]) == 3 {
			s = strings.ReplaceAll(s, ",", "")
		} else {
			s = strings.ReplaceAll(s, ",", ".")
		}
	}

	var price float64
	_, err := fmt.Sscanf(s, "%f", &price)
	if err != nil {
		return 0, fmt.Errorf("failed parsing price %s: %w", s, err)
	}
	return price, nil
}

func extractImage(doc *goquery.Document) string {
	// Look at landing image tag
	img := doc.Find("#landingImage").First()
	if src, exists := img.Attr("src"); exists {
		return src
	}
	if dataDynamic, exists := img.Attr("data-a-dynamic-image"); exists {
		// Just parse the first key out of the dict structure: {"URL1":[width,height],"URL2":[...]}
		// Since we don't want to import full JSON library just for string parse, let's extract the first URL
		idx := strings.Index(dataDynamic, `https://`)
		if idx != -1 {
			endIdx := strings.Index(dataDynamic[idx:], `"`)
			if endIdx != -1 {
				return dataDynamic[idx : idx+endIdx]
			}
		}
	}

	// Fallback to book cover
	if src, exists := doc.Find("#imgBlkFront").First().Attr("src"); exists {
		return src
	}

	return ""
}
