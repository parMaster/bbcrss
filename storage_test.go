package main

import (
	"context"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func SetupPgContainer(ctx context.Context, t *testing.T) (*DBConfig, error) {
	req := testcontainers.ContainerRequest{
		Image:        "postgres:14",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_PASSWORD": "password",
		},
		WaitingFor: wait.ForListeningPort("5432/tcp"),
	}
	postgresC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		log.Fatalf("Could not start rabbit: %s", err)
	}
	// Terminate the container when the test finishes
	go func() {
		<-ctx.Done()

		time.Sleep(2 * time.Second) // dirty hack to avoid "database system is shut down" error
		if err := postgresC.Terminate(ctx); err != nil {
			log.Printf("Could not stop postgres: %s", err)
		}
	}()

	host, err := postgresC.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := postgresC.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatal(err)
	}

	var cfg = DBConfig{
		Dsn:          fmt.Sprintf("host=%s port=%s user=postgres password=password dbname=postgres sslmode=disable", host, port.Port()),
		MaxOpenConns: 25,
		MaxIdleConns: 25,
		MaxIdleTime:  "15m",
	}

	return &cfg, nil
}

func migrateDb(cfg *DBConfig, act string) error {

	db, err := openDB(*cfg)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	log.Printf("database connection pool established")

	migrationDriver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("failed to create migration driver: %w", err)
	}

	migrator, err := migrate.NewWithDatabaseInstance("file://migrations/", "postgres", migrationDriver)
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}

	if act == "down" {
		err = migrator.Down()
	} else {
		err = migrator.Up()
	}
	if err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	log.Printf("database migrations applied")

	return nil
}

func TestWithPostgres(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := SetupPgContainer(ctx, t)
	assert.NoError(t, err)

	// Apply migrations
	err = migrateDb(cfg, "up")
	assert.NoError(t, err)

	// Migrate down on test finish
	defer func() {
		err := migrateDb(cfg, "down")
		if err != nil {
			log.Fatalf("failed to migrate down: %v", err)
		}
	}()

	store, err := NewStorage(*cfg)
	assert.NoError(t, err)
	defer store.Close()

	// Test CreateNewsItem

	// nil NewsItem
	err = store.CreateNewsItem(ctx, nil)
	assert.Error(t, err)

	// empty NewsItem
	err = store.CreateNewsItem(ctx, &NewsItem{
		Title: "",
		Link:  "",
	})
	assert.Error(t, err)

	// valid NewsItem
	validItem := NewsItem{
		Title:     "title",
		Link:      "link",
		Published: time.Now(),
	}
	err = store.CreateNewsItem(ctx, &validItem)
	assert.NoError(t, err)
	assert.NotZero(t, validItem.ID)

	// Create duplicate
	err = store.CreateNewsItem(ctx, &NewsItem{
		Title:     "title",
		Link:      "link",
		Published: time.Now(),
	})
	assert.ErrorIs(t, err, ErrAlreadyExists)

	// Test GetNewsItems
	dbItem, err := store.GetNewsItem(ctx, validItem.Link)
	assert.NoError(t, err)
	assert.Equal(t, validItem.Title, dbItem.Title)
	assert.Equal(t, validItem.Link, dbItem.Link)

	// Test SaveNewsItem
	validItem.Title = "updated_title"
	validItem.Link = "updated_link"
	validItem.Image = "image"
	validItem.Description = "description"
	err = store.SaveNewsItem(ctx, &validItem)
	assert.NoError(t, err)

	// Confirm the item was updated
	dbItem, err = store.GetNewsItem(ctx, validItem.Link)
	assert.NoError(t, err)
	assert.Equal(t, validItem.Title, dbItem.Title)
	assert.Equal(t, validItem.Link, dbItem.Link)
	assert.Equal(t, validItem.Image, dbItem.Image)
	assert.Equal(t, validItem.Description, dbItem.Description)

	// Test GetSingleNews
	_, err = store.GetSingleNews(ctx, 0)
	assert.ErrorIs(t, err, ErrNotFound)

	retrieved, err := store.GetSingleNews(ctx, validItem.ID)
	assert.NoError(t, err)

	assert.Equal(t, validItem.Title, retrieved.Title)
	assert.Equal(t, validItem.Link, retrieved.Link)
	assert.Equal(t, validItem.Image, retrieved.Image)
	assert.Equal(t, validItem.Description, retrieved.Description)

	// Create another item
	anotherItem := NewsItem{
		Title:     "another_title",
		Link:      "another_link",
		Published: time.Now(),
	}
	err = store.CreateNewsItem(ctx, &anotherItem)
	assert.NoError(t, err)

	// Test GetNews
	items, meta, err := store.GetNews(ctx, Filters{Page: 1, PageSize: 10})
	assert.NoError(t, err)
	assert.Len(t, items, 2)
	assert.Equal(t, 2, meta.TotalRecords)
	assert.Equal(t, 1, meta.CurrentPage)

	// Test GetNews with pagination
	items, meta, err = store.GetNews(ctx, Filters{Page: 1, PageSize: 1})
	assert.NoError(t, err)
	assert.Len(t, items, 1)
	assert.Equal(t, 2, meta.TotalRecords)
	assert.Equal(t, 1, meta.CurrentPage)

	items, meta, err = store.GetNews(ctx, Filters{Page: 2, PageSize: 1})
	assert.NoError(t, err)
	assert.Len(t, items, 1)
	assert.Equal(t, 2, meta.TotalRecords)
	assert.Equal(t, 2, meta.CurrentPage)

	items, meta, err = store.GetNews(ctx, Filters{Page: 3, PageSize: 1})
	assert.NoError(t, err)
	assert.Len(t, items, 0)
	assert.Equal(t, 0, meta.TotalRecords) // rows.Next() returned false
	assert.Equal(t, 0, meta.CurrentPage)

}
