// TODO: run only if -integration flag is set
// Integration tests for api.go
package main

import (
	"context"
	"testing"

	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

func defaultCfg() *Config {
	var cfg Config
	p := flags.NewParser(&cfg, flags.IgnoreUnknown)
	p.Parse()
	return &cfg
}

// Test_ListAndSingleNews tests listNews and getSingleNews functions
func Test_ListAndSingleNews(t *testing.T) {
	cfg := defaultCfg()
	// TODO: this is not good
	cfg.DB.Dsn = "postgres://bbcrss:hunter2@mini/bbcrss?sslmode=disable"

	s, err := NewService(cfg)
	assert.NoError(t, err)

	api, err := NewAPIServer(s.Storage, cfg.API)
	assert.NoError(t, err)

	list, meta, err := api.listNews(context.Background(), Filters{})
	assert.NoError(t, err)
	assert.NotNil(t, list)
	assert.NotNil(t, meta)

	assert.Equal(t, 5, len(list))
	assert.Equal(t, 5, meta.PageSize)

	// get single news
	singleNews, err := api.getSingleNews(context.Background(), list[0].ID)
	assert.NoError(t, err)
	assert.NotNil(t, singleNews)
}
