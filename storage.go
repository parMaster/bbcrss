package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"

	"github.com/lib/pq"
)

var (
	// ErrAlreadyExists is returned when item already exists in DB (unique constraint violation)
	ErrAlreadyExists = errors.New("item already exists")
)

// Storage is responsible for CRUD operations with DB for news items
type Storage struct {
	db *sql.DB
}

func NewStorage(cfg *Config) (*Storage, error) {
	db, err := openDB(cfg)
	if err != nil {
		return nil, err
	}

	return &Storage{db: db}, nil
}

// CreateNewsItem saves news item to DB. Minimum required fields are Title and Link
func (s *Storage) CreateNewsItem(ctx context.Context, item *NewsItem) error {

	if item == nil || item.Title == "" || item.Link == "" {
		return errors.New("item is empty")
	}

	args := []any{item.Title, item.Link, item.Published}

	err := s.db.QueryRowContext(ctx,
		`INSERT INTO news (title, link, published)
		VALUES ($1, $2, $3::timestamp)
		RETURNING id`,
		args...).Scan(&item.ID)

	if err != nil {
		pgErr, ok := err.(*pq.Error)
		// check if item already exists, return special error
		if ok && pgErr.Code == "23505" {
			return ErrAlreadyExists
		}
	}

	return err
}

// GetNewsItem returns news item by Link
func (s *Storage) GetNewsItem(ctx context.Context, link string) (*NewsItem, error) {
	item := NewsItem{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, title, link, published, description, image FROM news WHERE link = $1`,
		link).Scan(&item.ID, &item.Title, &item.Link, &item.Published, &item.Description, &item.Image)

	if err != nil {
		return nil, err
	}

	return &item, nil
}

// SaveNewsItem updates news item in DB
func (s *Storage) SaveNewsItem(ctx context.Context, item *NewsItem) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE news SET title = $1, link = $2, description = $3, image = $4 WHERE id = $5`,
		item.Title, item.Link, item.Description, item.Image, item.ID)

	return err
}

func (s *Storage) GetNews(ctx context.Context) ([]NewsItem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, link, published, description, image FROM news`)
	if err != nil {
		return nil, err
	}
	defer func() {
		err := rows.Close()
		if err != nil {
			log.Printf("failed to close rows: %v", err)
		}
	}()

	items := []NewsItem{}
	for rows.Next() {
		item := NewsItem{}
		err = rows.Scan(&item.ID, &item.Title, &item.Link, &item.Published, &item.Description, &item.Image)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, nil
}

// Close closes DB connection
func (s *Storage) Close() error {
	return s.db.Close()
}

// openDB opens connection to PostgreSQL DB, pings it and returns connection
func openDB(cfg *Config) (*sql.DB, error) {
	db, err := sql.Open("postgres", cfg.DB.Dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(cfg.DB.MaxOpenConns)
	db.SetMaxIdleConns(cfg.DB.MaxIdleConns)

	duration, err := time.ParseDuration(cfg.DB.MaxIdleTime)
	if err != nil {
		return nil, err
	}

	db.SetConnMaxIdleTime(duration)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = db.PingContext(ctx)
	if err != nil {
		return nil, err
	}

	return db, nil
}
