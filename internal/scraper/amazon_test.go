package scraper

import (
	"testing"
)

func TestParseAmazonURL(t *testing.T) {
	tests := []struct {
		name         string
		rawURL       string
		expectedASIN string
		expectedURL  string
		expectError  bool
	}{
		{
			name:         "Standard Indian Product URL",
			rawURL:       "https://www.amazon.in/dp/B0CXM1K4TL",
			expectedASIN: "B0CXM1K4TL",
			expectedURL:  "https://www.amazon.in/dp/B0CXM1K4TL",
			expectError:  false,
		},
		{
			name:         "US Product URL with Title and Query Params",
			rawURL:       "https://www.amazon.com/Apple-iPhone-15-Pro-Max/dp/B0CXM1K4TL/ref=sr_1_3?crid=123&qid=456",
			expectedASIN: "B0CXM1K4TL",
			expectedURL:  "https://www.amazon.com/dp/B0CXM1K4TL",
			expectError:  false,
		},
		{
			name:         "UK Product URL with gp/product structure",
			rawURL:       "https://www.amazon.co.uk/gp/product/B08N5WRWNW?psc=1",
			expectedASIN: "B08N5WRWNW",
			expectedURL:  "https://www.amazon.co.uk/dp/B08N5WRWNW",
			expectError:  false,
		},
		{
			name:         "Mobile URL gp/aw/d",
			rawURL:       "https://www.amazon.com/gp/aw/d/B0CXM1K4TL",
			expectedASIN: "B0CXM1K4TL",
			expectedURL:  "https://www.amazon.com/dp/B0CXM1K4TL",
			expectError:  false,
		},
		{
			name:         "Non-Amazon URL",
			rawURL:       "https://www.google.com/search?q=iphone",
			expectedASIN: "",
			expectedURL:  "",
			expectError:  true,
		},
		{
			name:         "No ASIN in Amazon URL",
			rawURL:       "https://www.amazon.com/gp/help/customer/display.html",
			expectedASIN: "",
			expectedURL:  "",
			expectError:  true,
		},
		{
			name:         "Amazon India Shortened Link amzn.in/d/...",
			rawURL:       "https://amzn.in/d/0gAGaNav",
			expectedASIN: "B0DWDQYB87",
			expectedURL:  "https://www.amazon.in/dp/B0DWDQYB87",
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			asin, normURL, err := ParseAmazonURL(tt.rawURL)
			if (err != nil) != tt.expectError {
				t.Fatalf("expected error: %v, got: %v", tt.expectError, err)
			}
			if !tt.expectError {
				if asin != tt.expectedASIN {
					t.Errorf("expected ASIN %s, got %s", tt.expectedASIN, asin)
				}
				if normURL != tt.expectedURL {
					t.Errorf("expected URL %s, got %s", tt.expectedURL, normURL)
				}
			}
		})
	}
}
