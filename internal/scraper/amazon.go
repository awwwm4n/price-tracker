package scraper

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Product represents the details scraped from an Amazon product page.
type Product struct {
	ASIN     string
	Title    string
	Price    float64
	ImageURL string
	URL      string
}

// Scraper defines the interface for scraping Amazon products.
type Scraper interface {
	Scrape(ctx context.Context, rawURL string) (*Product, error)
}

// asinRegex extracts 10-character alphanumeric Amazon ASIN.
var asinRegex = regexp.MustCompile(`/(?:dp|gp/product|gp/aw/d|d)/([A-Z0-9]{10})`)

// ParseAmazonURL extracts the ASIN and returns a clean, normalized Amazon URL.
func ParseAmazonURL(rawURL string) (asin string, normalizedURL string, err error) {
	// Clean string
	rawURL = strings.TrimSpace(rawURL)
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "https://" + rawURL
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", fmt.Errorf("invalid URL structure: %w", err)
	}

	// Validate it's an Amazon site or shortener
	host := strings.ToLower(u.Host)
	if strings.Contains(host, "amzn.") {
		// Resolve redirects for shortened links (e.g., amzn.in, amzn.to)
		resolvedURL, err := resolveRedirects(rawURL)
		if err != nil {
			return "", "", fmt.Errorf("failed to resolve short URL: %w", err)
		}
		return ParseAmazonURL(resolvedURL)
	}

	if !strings.Contains(host, "amazon.") {
		return "", "", fmt.Errorf("not an Amazon domain: %s", u.Host)
	}

	// Find the top level domain (Tld) like .in, .com, .co.uk
	// e.g. "www.amazon.co.uk" -> "amazon.co.uk", "www.amazon.in" -> "amazon.in"
	parts := strings.Split(host, "amazon.")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("could not parse Amazon host: %s", host)
	}
	tld := parts[1]

	// Find the ASIN in the path
	path := u.Path
	// Some links might place the ASIN after a parameter, check raw path too
	matches := asinRegex.FindStringSubmatch(path)
	if len(matches) < 2 {
		// If path doesn't match, check the full URL query or raw path
		matches = asinRegex.FindStringSubmatch(rawURL)
		if len(matches) < 2 {
			return "", "", fmt.Errorf("could not extract ASIN from URL: %s", rawURL)
		}
	}

	asin = strings.ToUpper(matches[1])
	normalizedURL = fmt.Sprintf("https://www.amazon.%s/dp/%s", tld, asin)
	return asin, normalizedURL, nil
}

func resolveRedirects(rawURL string) (string, error) {
	currentURL := rawURL
	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Do not follow redirects automatically
			return http.ErrUseLastResponse
		},
	}

	// Max 5 redirect hops
	for i := 0; i < 5; i++ {
		req, err := http.NewRequest("GET", currentURL, nil)
		if err != nil {
			return "", err
		}
		// Use realistic User-Agent to avoid blocks on shorteners
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

		resp, err := client.Do(req)
		if err != nil {
			return "", fmt.Errorf("network connection error on redirect: %w", err)
		}
		resp.Body.Close() // Close immediately

		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			loc := resp.Header.Get("Location")
			if loc == "" {
				return currentURL, nil
			}

			u, err := url.Parse(loc)
			if err != nil {
				return "", fmt.Errorf("invalid redirect location URL: %w", err)
			}
			if !u.IsAbs() {
				base, _ := url.Parse(currentURL)
				currentURL = base.ResolveReference(u).String()
			} else {
				currentURL = loc
			}
			continue
		}

		// Not a redirect status, we reached the destination
		break
	}

	return currentURL, nil
}
