package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/mmcdole/gofeed"
)

// Parser is responsible for parsing RSS feed into slice of items
type Parser struct {
	cfg *Config
}

// NewParser constructs new Parser
func NewParser(cfg *Config) *Parser {
	return &Parser{cfg: cfg}
}

// getContents fetches feed as a string from given URL
func (p *Parser) getContents(ctx context.Context, url string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.3")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get feed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read body: %w", err)
	}

	return string(body), nil
}

// parseRSS reads RSS feed and returns slice of news items or error.
// Only Title and Link are extracted
func (p *Parser) parseRSS(feedBody string) ([]NewsItem, error) {
	items := []NewsItem{}

	fp := gofeed.NewParser()
	feed, err := fp.ParseString(feedBody)
	if err != nil {
		return nil, fmt.Errorf("failed to parse RSS feed: %w", err)
	}

	for _, item := range feed.Items {
		// log.Printf("item %d: %+v \n\n", i, item)
		items = append(items, NewsItem{Title: item.Title, Link: item.Link})
	}

	return items, nil
}

// GetNews fetches RSS feed, parses it and returns slice of news items or error
func (p *Parser) GetNews(ctx context.Context) ([]NewsItem, error) {
	feedBody, err := p.getContents(ctx, p.cfg.RssUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to get feed: %w", err)
	}

	items, err := p.parseRSS(feedBody)
	if err != nil {
		return nil, fmt.Errorf("failed to parse RSS: %w", err)
	}

	return items, nil
}

// Enrich fetches link contents and extracts enrichment data into NewsItem
func (p *Parser) Enrich(ctx context.Context, item *NewsItem) (int, error) {
	enrichments, err := p.GetEnrichments(ctx, item.Link)
	if err != nil {
		return 0, fmt.Errorf("failed to get enrichments: %w", err)
	}

	item.Description = enrichments["description"]
	item.Image = enrichments["image"]

	return len(enrichments), nil
}

// getEnrichments fetches link contents and extracts enrichment data
func (p *Parser) GetEnrichments(ctx context.Context, link string) (map[string]string, error) {
	body, err := p.getContents(ctx, link)
	if err != nil {
		return nil, fmt.Errorf("failed to get enrichments: %w", err)
	}

	enrichments, err := p.extractEnrichments(body)
	if err != nil {
		return nil, fmt.Errorf("failed to extract enrichments: %w", err)
	}

	return enrichments, nil
}

// enrichmentTable is a map of name:regexp pairs for enrichment
var enrichmentTable = map[string]string{
	"description": `(?i)<meta[^>]+name="description"[^>]+content="([^"]+)"`,
	"image":       `(?i)<meta[^>]+property="og:image"[^>]+content="([^"]+)"`,
}

// extractEnrichments extracts enrichment data from HTML
func (p *Parser) extractEnrichments(html string) (map[string]string, error) {
	enrichments := make(map[string]string)
	for name, re := range enrichmentTable {
		matches := regexp.MustCompile(re).FindStringSubmatch(html)
		if len(matches) > 1 {
			enrichments[name] = matches[1]
		}
	}

	return enrichments, nil
}
