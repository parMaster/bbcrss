package main

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-pkgz/rest"
)

//go:embed all:web
var web embed.FS

// Storer is an interface for storage
type Storer interface {
	GetNews(ctx context.Context, filters Filters) ([]NewsItem, Metadata, error)
	GetSingleNews(ctx context.Context, id int) (*NewsItem, error)
}

// APIServer ..
type APIServer struct {
	Storage Storer
	cfg     APIConfig
}

// NewServer creates new API server
func NewAPIServer(storage Storer, cfg APIConfig) (*APIServer, error) {
	return &APIServer{
		Storage: storage,
		cfg:     cfg,
	}, nil
}

func (api *APIServer) Run(ctx context.Context) error {
	httpServer := &http.Server{
		Addr:              api.cfg.Listen,
		Handler:           api.router(ctx),
		ReadHeaderTimeout: time.Second,
		ReadTimeout:       time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       time.Second,
	}

	err := httpServer.ListenAndServe()
	if err != nil {
		return fmt.Errorf("failed to start http server: %w", err)
	}
	log.Printf("http server started on %s", api.cfg.Listen)

	<-ctx.Done()
	log.Printf("Terminating http server")

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("[ERROR] shutting down http server: %v", err)
		return fmt.Errorf("[ERROR] shutting down http server: %w", err)
	}
	return nil
}

func (api *APIServer) listNews(ctx context.Context, filters Filters) ([]NewsItem, Metadata, error) {
	filters.validate(defaultFilters)

	items, meta, err := api.Storage.GetNews(ctx, filters)
	if err != nil {
		log.Printf("failed to get news: %v", err)
		return nil, Metadata{}, err
	}

	return items, meta, nil
}

func (api *APIServer) getSingleNews(ctx context.Context, id int) (*NewsItem, error) {
	item, err := api.Storage.GetSingleNews(ctx, id)
	if err != nil {
		log.Printf("failed to get single news: %v", err)
		return nil, err
	}

	return item, nil
}

// API handlers

// router creates http router
func (api *APIServer) router(ctx context.Context) http.Handler {
	router := chi.NewRouter()
	router.Use(rest.Throttle(5))

	// Web UI
	router.Get("/", api.indexHandler(ctx))
	router.Get("/article", api.articleHandler(ctx))

	// REST API
	router.Get("/v1/news", api.listNewsHandler(ctx))

	return router
}

// Web UI handlers

var funcMap = template.FuncMap{
	"sub":      func(a, b int) int { return a - b },
	"add":      func(a, b int) int { return a + b },
	"dateStr":  func(t time.Time) string { return t.Format("January 2, 2006 15:04") },
	"unescape": func(s string) template.HTML { return template.HTML(s) },
}

// indexHandler renders index page
func (api *APIServer) indexHandler(ctx context.Context) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {

		// parse paging parameters
		filters := Filters{}
		pageStr := r.URL.Query().Get("page")
		filters.Page, _ = strconv.Atoi(pageStr)

		pageSizeStr := r.URL.Query().Get("pagesize")
		filters.PageSize, _ = strconv.Atoi(pageSizeStr)

		// validate by fallback to default, don`t yell on user, show something
		filters.validate(defaultFilters)

		news, meta, err := api.listNews(ctx, filters)
		if err != nil {
			log.Printf("failed to get listNews: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		envelope := struct {
			News     []NewsItem
			Metadata Metadata
		}{
			News:     news,
			Metadata: meta,
		}

		// load ./web/index.html as a template
		tpl := template.Must(template.New("index.html").Funcs(funcMap).ParseFS(web, "web/index.html"))

		// render it with data from envelope

		err = tpl.Execute(w, envelope)
		if err != nil {
			log.Printf("failed to render template: %v", err)
			return
		}
	}
}

// artcileHandler renders single article page
func (api *APIServer) articleHandler(ctx context.Context) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := r.URL.Query().Get("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			log.Printf("failed to parse id: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		item, err := api.getSingleNews(ctx, id)
		if err != nil {
			log.Printf("failed to get single news: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		tpl := template.Must(template.New("article.html").Funcs(funcMap).ParseFS(web, "web/article.html"))
		err = tpl.Execute(w, item)
		if err != nil {
			log.Printf("failed to render template: %v", err)
			return
		}
	}
}

// REST API handlers

// listNewsHandler returns JSON list of news items
func (api *APIServer) listNewsHandler(ctx context.Context) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		filters := Filters{}
		pageStr := chi.URLParam(r, "page")
		page, err := strconv.Atoi(pageStr)
		if err != nil {
			log.Printf("failed to parse page: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		pageSizeStr := chi.URLParam(r, "pagesize")
		pageSize, err := strconv.Atoi(pageSizeStr)
		if err != nil {
			log.Printf("failed to parse page size: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		filters.Page = page
		filters.PageSize = pageSize

		news, meta, err := api.listNews(ctx, filters)
		if err != nil {
			log.Printf("failed to get listNews: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		envelope := map[string]interface{}{
			"news":     news,
			"metadata": meta,
		}

		rest.RenderJSON(w, envelope)
	}
}
