package scraper

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestParsePriceString(t *testing.T) {
	tests := []struct {
		name          string
		rawPrice      string
		expectedPrice float64
		expectError   bool
	}{
		{"Clean USD", "$19.99", 19.99, false},
		{"Rupees with symbol and commas", "₹ 54,999.00", 54999.00, false},
		{"Decimal comma European", "19,99", 19.99, false},
		{"Thousand dot and decimal comma", "1.299,99", 1299.99, false},
		{"Rupees thousands separator only", "₹54,999", 54999.00, false},
		{"No numeric value", "Out of stock", 0.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			price, err := parsePriceString(tt.rawPrice)
			if (err != nil) != tt.expectError {
				t.Fatalf("expected error: %v, got: %v", tt.expectError, err)
			}
			if !tt.expectError && price != tt.expectedPrice {
				t.Errorf("expected price %f, got %f", tt.expectedPrice, price)
			}
		})
	}
}

func TestExtractPriceFromHTML(t *testing.T) {
	htmlData := `
	<html>
		<body>
			<span class="priceToPay">
				<span class="a-offscreen">₹54,999.00</span>
			</span>
			<div id="productTitle">Apple iPhone 15</div>
			<img id="landingImage" src="https://images.amazon.com/iphone15.jpg" />
		</body>
	</html>
	`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlData))
	if err != nil {
		t.Fatalf("failed to create document: %v", err)
	}

	price, err := extractPrice(doc)
	if err != nil {
		t.Fatalf("failed to extract price: %v", err)
	}

	if price != 54999.00 {
		t.Errorf("expected extracted price to be 54999.00, got %.2f", price)
	}

	title := strings.TrimSpace(doc.Find("#productTitle").First().Text())
	if title != "Apple iPhone 15" {
		t.Errorf("expected title to be 'Apple iPhone 15', got '%s'", title)
	}

	image := extractImage(doc)
	if image != "https://images.amazon.com/iphone15.jpg" {
		t.Errorf("expected image URL to be 'https://images.amazon.com/iphone15.jpg', got '%s'", image)
	}
}
