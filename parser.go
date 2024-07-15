package main

import (
	"context"
	"fmt"
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

// ParseRSS reads RSS feed and returns slice of items
func (p *Parser) ParseRSS(ctx context.Context) ([]NewsItem, error) {
	items := []NewsItem{}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	fp := gofeed.NewParser()
	feed, err := fp.ParseURLWithContext(p.cfg.RssUrl, ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to parse RSS feed: %w", err)
	}

	for _, item := range feed.Items {
		// log.Printf("item %d: %+v \n\n", i, item)
		items = append(items, NewsItem{Title: item.Title, Link: item.Link})
	}

	return items, nil
}
