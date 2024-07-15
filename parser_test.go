package main

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

var validRssFeeds = []string{
	"https://www.reddit.com/r/golang.rss",
	"https://feeds.bbci.co.uk/news/world/rss.xml",
	"http://feeds.twit.tv/twit.xml",
}

// Test_GetContents tests error handling in getFeed
func Test_GetContents(t *testing.T) {

	cases := []struct {
		url string
		ok  bool
	}{
		{"http://localhost:12345/invalid", false},
		{"error", false},
		{"", false},
		{"https://feeds.bbci.co.uk/news/world/rss.xml", true},
		{"https://google.com", true}, // not a feed, but should return 200
	}

	ctx := context.Background()
	p := Parser{}

	for _, tc := range cases {
		feed, err := p.getContents(ctx, tc.url)
		if tc.ok {
			assert.NoError(t, err)
			assert.NotEmpty(t, feed)
		} else {
			assert.Error(t, err)
			assert.Empty(t, feed)
		}
	}
}

// Test_ParseRSS tests error handling in parseRSS
func Test_ParseRSS(t *testing.T) {

	cases := []struct {
		name     string
		feed     string
		empty    bool
		valid    bool
		expItems int
	}{
		{"invalid", "", true, false, 0},
		{"empty", "<rss></rss>", true, true, 0}, // empty feed ([]items), but valid
		{"single", `<rss version="2.0">
		<channel>
			<title>Test channel</title>
			<item>
				<title>Test item</title>
				<link>http://example.com</link>
			</item>
		</channel>
		</rss>`, false, true, 1},
		{"double", `<rss version="2.0">
		<channel>
			<title>Test channel</title>
			<item>
				<title>Test item</title>
				<link>http://example.com</link>
			</item>
			<item>
				<title>Test item</title>
				<link>http://example.com</link>
			</item>
		</channel>
		</rss>`, false, true, 2},
		{"not an rss", `<rpfs version="2.0">
		<channel>
			<title>Test channel</title>
			<item>
				<title>Test item</title>
				<link>http://example.com</link>
			</item>
		</channel>
		</rss>`, true, false, 0},
		{"not an rss as well", `<xml version="2.0">
		</rss>`, true, false, 0},
	}

	p := Parser{}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items, err := p.parseRSS(tc.feed)
			if tc.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}

			if tc.empty {
				assert.Empty(t, items)
			} else {
				assert.NotEmpty(t, items)
				assert.Equal(t, tc.expItems, len(items))
			}

		})

	}
}

// getFeed, parseRSS and GetNews are tested together. Happy path only
// kind of integration test
func Test_GetAndParse(t *testing.T) {

	ctx := context.Background()

	for _, rssFeed := range validRssFeeds {

		cfg := &Config{
			RssUrl: rssFeed, // everything parser needs to know
		}

		p := NewParser(cfg)
		feed, err := p.getContents(ctx, rssFeed)
		assert.NoError(t, err)
		assert.NotEmpty(t, feed)

		// try to parse feed, since we've already got the RSS body
		items, err := p.parseRSS(feed)
		assert.NoError(t, err)
		assert.NotEmpty(t, items)

		newsItems, err := p.GetNews(ctx)
		assert.NoError(t, err)
		assert.NotEmpty(t, newsItems)

		assert.True(t, reflect.DeepEqual(items, newsItems))
	}
}

func Test_ExtractEnrichments(t *testing.T) {

	cases := []struct {
		name string
		body string
		exp  map[string]string
	}{
		{"empty", "", map[string]string{}},
		{"no meta", "<html></html>", map[string]string{}},
		{"description", `<html>
		<meta name="description" content="test description">
		</html>`, map[string]string{"description": "test description"}},
		{"image", `<html>
		<meta property="og:image" content="http://example.com/image.jpg">
		</html>`, map[string]string{"image": "http://example.com/image.jpg"}},
		{"both", `<html>
		<meta name="description" content="test description">
		<meta property="og:image" content="http://example.com/image.jpg">
		</html>`, map[string]string{"description": "test description", "image": "http://example.com/image.jpg"}},
	}

	p := Parser{}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			enrichments, err := p.extractEnrichments(tc.body)
			assert.NoError(t, err)
			assert.Equal(t, tc.exp, enrichments)
		})
	}

}

// fetching and parsing feed, then enriching items
func Test_ParseRssAndEnrich(t *testing.T) {

	ctx := context.Background()

	rssFeed := "https://feeds.bbci.co.uk/news/world/rss.xml"

	cfg := &Config{
		RssUrl: rssFeed, // everything parser needs to know
	}

	p := NewParser(cfg)
	feed, err := p.getContents(ctx, rssFeed)
	assert.NoError(t, err)
	assert.NotEmpty(t, feed)

	// try to parse feed, since we've already got the RSS body
	items, err := p.parseRSS(feed)
	assert.NoError(t, err)
	assert.NotEmpty(t, items)

	for _, item := range items {
		applied, err := p.Enrich(ctx, &item)
		assert.NoError(t, err)
		assert.Equal(t, 2, applied) // 2 enrichments applied
		// check if enrichments are in fact applied
		assert.NotEmpty(t, item.Description)
		assert.NotEmpty(t, item.Image)
	}
}
