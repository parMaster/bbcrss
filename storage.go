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
	// ErrNotFound is returned when item not found in DB
	ErrNotFound = errors.New("item not found")
)

// Storage is responsible for CRUD operations with DB for news items
type Storage struct {
	db *sql.DB
}

// Factory function to create new Storage
func NewStorage(cfg DBConfig) (*Storage, error) {
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
		link).Scan(
		&item.ID,
		&item.Title,
		&item.Link,
		&item.Published,
		&item.Description,
		&item.Image)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &item, nil
}

// SaveNewsItem updates news item in DB
func (s *Storage) SaveNewsItem(ctx context.Context, item *NewsItem) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE news SET title = $1, link = $2, description = $3, image = $4 WHERE id = $5
		RETURNING id
		`,
		item.Title,
		item.Link,
		item.Description,
		item.Image,
		item.ID)

	if err != nil {
		return err
	}

	affected, err := res.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}

	return err
}

func (s *Storage) GetNews(ctx context.Context, filters Filters) ([]NewsItem, Metadata, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx,
		`SELECT count(*) OVER(), id, title, link, published, description, image 
		FROM news
		ORDER BY published DESC
		LIMIT $1 OFFSET $2
		`, filters.limit(), filters.offset())
	if err != nil {
		return nil, Metadata{}, err
	}
	defer func() {
		err := rows.Close()
		if err != nil {
			log.Printf("failed to close rows: %v", err)
		}
	}()

	totalRecords := 0
	items := []NewsItem{}
	for rows.Next() {
		item := NewsItem{}
		err = rows.Scan(
			&totalRecords,
			&item.ID,
			&item.Title,
			&item.Link,
			&item.Published,
			&item.Description,
			&item.Image,
		)
		if err != nil {
			return nil, Metadata{}, err
		}
		items = append(items, item)
	}

	metadata := calculateMetadata(totalRecords, filters.Page, filters.PageSize)

	return items, metadata, nil
}

// GetNewsItem returns news item by Link
func (s *Storage) GetSingleNews(ctx context.Context, id int) (*NewsItem, error) {
	item := NewsItem{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, title, link, published, description, image FROM news WHERE id = $1`,
		id).Scan(
		&item.ID,
		&item.Title,
		&item.Link,
		&item.Published,
		&item.Description,
		&item.Image)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &item, nil
}

// Close closes DB connection
func (s *Storage) Close() error {
	return s.db.Close()
}

// openDB opens connection to PostgreSQL DB, pings it and returns connection
func openDB(cfg DBConfig) (*sql.DB, error) {
	db, err := sql.Open("postgres", cfg.Dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)

	duration, err := time.ParseDuration(cfg.MaxIdleTime)
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
