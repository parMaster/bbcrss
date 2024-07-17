// Integration tests for api.go
package main

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go/modules/rabbitmq"
)

func SetupRmqContainer(ctx context.Context, t *testing.T) (*RMQConfig, error) {

	rabbitmqContainer, err := rabbitmq.Run(ctx,
		"rabbitmq:3.7.25-management-alpine",
		rabbitmq.WithAdminUsername("admin"),
		rabbitmq.WithAdminPassword("password"),
	)
	if err != nil {
		log.Fatalf("failed to start container: %s", err)
	}
	// Terminate the container when the test finishes
	go func() {
		<-ctx.Done()

		if err := rabbitmqContainer.Terminate(ctx); err != nil {
			log.Printf("Could not stop rabbit: %s", err)
		}
	}()

	amqpURL, err := rabbitmqContainer.AmqpURL(ctx)
	if err != nil {
		log.Fatalf("failed to get AMQP URL: %s", err) // nolint:gocritic
	}

	var cfg = RMQConfig{
		Dsn:  amqpURL,
		Name: "test",
	}

	return &cfg, nil
}

func Setup(ctx context.Context, t *testing.T) (*Config, error) {
	cfg := Config{
		RssUrl: "https://feeds.bbci.co.uk/news/world/rss.xml",
	}

	// Setup Postgres container
	dbCfg, err := SetupPgContainer(ctx, t)
	if err != nil {
		return nil, err
	}

	err = migrateDb(dbCfg, "up")
	if err != nil {
		return nil, err
	}
	cfg.DB = *dbCfg

	// Migrate down on context cancel
	go func() {
		<-ctx.Done()
		err := migrateDb(dbCfg, "down")
		if err != nil {
			log.Fatalf("failed to migrate down: %v", err)
		}
	}()

	// Setup RabbitMQ container
	rmqCfg, err := SetupRmqContainer(ctx, t)
	if err != nil {
		return nil, err
	}
	cfg.RMQ = *rmqCfg

	return &cfg, nil
}

// Test_LoadEnrichList tests loading news, enriching and listing them using
// listNews and getSingleNews functions
func Test_LoadEnrichList(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := Setup(ctx, t)
	assert.NoError(t, err)

	s, err := NewService(cfg)
	assert.NoError(t, err)

	// Initial news load
	items, err := s.Parser.GetNews(ctx)
	assert.NoError(t, err)

	saved := 0
	for _, item := range items {
		err := s.Storage.CreateNewsItem(ctx, &item)
		if err == ErrAlreadyExists {
			continue
		}
		saved++
		assert.NoError(t, err)

		if err == nil {
			err := s.EnrichNewsItem(ctx, item.Link)
			assert.NoError(t, err)
		}
	}

	// API server
	cfg.API = APIConfig{
		Listen: ":9980",
	}

	api, err := NewAPIServer(s.Storage, cfg.API)
	assert.NoError(t, err)

	// Test listNews with default filters
	list, meta, err := api.listNews(context.Background(),
		Filters{defaultFilters.Page, defaultFilters.PageSize})
	assert.NoError(t, err)
	assert.NotNil(t, list)
	assert.NotNil(t, meta)

	assert.Equal(t, saved, meta.TotalRecords)
	assert.Equal(t, defaultFilters.PageSize, len(list)) // correct page size

	// Test loading second page
	list2, meta, err := api.listNews(context.Background(),
		Filters{defaultFilters.Page + 1, defaultFilters.PageSize})
	assert.NoError(t, err)
	assert.NotNil(t, list2)
	assert.NotNil(t, meta)

	assert.Equal(t, saved, meta.TotalRecords)
	assert.Equal(t, defaultFilters.PageSize, len(list)) // correct page size

	assert.NotEqual(t, list[0].ID, list2[0].ID)       // different items
	assert.NotEqual(t, list[0].Title, list2[0].Title) // different items

	// Test listNews, filter all news
	listAll, meta, err := api.listNews(context.Background(),
		Filters{defaultFilters.Page, meta.TotalRecords + 10})
	assert.NoError(t, err)
	assert.NotNil(t, listAll)
	assert.NotNil(t, meta)

	assert.Equal(t, meta.TotalRecords, len(listAll))

	// Test get single news
	singleNews, err := api.getSingleNews(context.Background(), listAll[0].ID)
	assert.NoError(t, err)
	assert.NotNil(t, singleNews)

	// validate content
	assert.Equal(t, listAll[0].ID, singleNews.ID)
	assert.Equal(t, listAll[0].Title, singleNews.Title)
	assert.Equal(t, listAll[0].Link, singleNews.Link)
	assert.Equal(t, listAll[0].Description, singleNews.Description)

	// Test listNews, filter over limit
	listEmpty, meta, err := api.listNews(context.Background(),
		Filters{defaultFilters.Page + 1, meta.TotalRecords + 10})
	assert.NoError(t, err)
	assert.Equal(t, []NewsItem{}, listEmpty)
	assert.Equal(t, Metadata{}, meta)

	// Test get single news with invalid ID
	singleNews, err = api.getSingleNews(context.Background(), 0)
	assert.ErrorIs(t, ErrNotFound, err)
	assert.Nil(t, singleNews)

	// Since the data is loaded and containers are running, we can test the API handlers

	// Test index handler
	indexHandler := s.ApiServer.indexHandler(ctx)

	// Test first page
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	indexHandler(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body := w.Body.String()
	assert.True(t, strings.Contains(body, "Latest News"), "Title should be present")
	assert.Equal(t, defaultFilters.PageSize, strings.Count(body, "<div class=\"news-item\">"), "News items count should be equal to page size")

	// News items should be present on the page
	for _, item := range list {
		assert.True(t, strings.Contains(body, string(template.HTML(item.Title))), "Title should be present")             // unescaped
		assert.True(t, strings.Contains(body, string(template.HTML(item.Description))), "Description should be present") // unescaped
		assert.True(t, strings.Contains(body, item.Image), "Image URL should be present")
	}

	// Test second page
	req = httptest.NewRequest("GET", "/?page=2", nil) // default page size is 5
	w = httptest.NewRecorder()
	indexHandler(w, req)
	resp = w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body = w.Body.String()
	assert.Equal(t, defaultFilters.PageSize, strings.Count(body, "<div class=\"news-item\">"))

	// News items should be present on the page
	for _, item := range list2 {
		assert.True(t, strings.Contains(body, string(template.HTML(item.Title))), "Title should be present")             // unescaped
		assert.True(t, strings.Contains(body, string(template.HTML(item.Description))), "Description should be present") // unescaped
		assert.True(t, strings.Contains(body, item.Image), "Image URL should be present")
	}

	// Test All news
	req = httptest.NewRequest("GET", "/?page=1&pagesize=100", nil)
	w = httptest.NewRecorder()
	indexHandler(w, req)
	resp = w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, saved, strings.Count(w.Body.String(), "<div class=\"news-item\">"))

	// Test page over limit
	req = httptest.NewRequest("GET", "/?page=2&pagesize=100", nil)
	w = httptest.NewRecorder()
	indexHandler(w, req)
	resp = w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 0, strings.Count(w.Body.String(), "<div class=\"news-item\">"), "No news should be displayed")
	assert.True(t, strings.Contains(w.Body.String(), "No news available."), "No news available. message should be displayed")

	// Test article handler
	articleHandler := s.ApiServer.articleHandler(ctx)

	// Test article handler with invalid numeric ID should return http.StatusNotFound
	req = httptest.NewRequest("GET", "/article?id=0", nil)
	w = httptest.NewRecorder()
	articleHandler(w, req)
	resp = w.Result()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "Should return 404")

	// Test article handler with invalid ID (non-numeric) - should return http.StatusBadRequest
	req = httptest.NewRequest("GET", "/article?id=invalid", nil)
	w = httptest.NewRecorder()
	articleHandler(w, req)
	resp = w.Result()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Should return 400")

	// Test article handler with valid ID
	req = httptest.NewRequest("GET", fmt.Sprintf("/article?id=%d", listAll[0].ID), nil)
	w = httptest.NewRecorder()
	articleHandler(w, req)
	resp = w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode, "Should return 200")
	// check the content
	body = w.Body.String()
	assert.True(t, strings.Contains(body, string(template.HTML(listAll[0].Title))), "Title should be present")
	assert.True(t, strings.Contains(body, string(template.HTML(listAll[0].Description))), "Description should be present")
	assert.True(t, strings.Contains(body, listAll[0].Image), "Image URL should be present")

}
