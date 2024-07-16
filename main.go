package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-pkgz/lgr"
	"github.com/jessevdk/go-flags"
)

type Config struct {
	Dbg    bool      `long:"dbg" env:"DBG" description:"debug mode, more verbose output"`
	RssUrl string    `long:"rss" env:"RSS" default:"https://feeds.bbci.co.uk/news/world/rss.xml" description:"RSS news feed URL"`
	RssTtl string    `long:"rss-ttl" env:"RSS_TTL" default:"15m" description:"RSS feed TTL"`
	DB     DBConfig  `group:"DB Config"`
	RMQ    RMQConfig `group:"RMQ Config"`
	API    APIConfig `group:"API Config"`
}

type RMQConfig struct {
	Dsn  string `long:"rmq-dsn" env:"RMQ_DSN" default:"amqp://guest:guest@localhost:5672/" description:"RabbitMQ DSN"`
	Name string `long:"rmq-name" env:"RMQ_NAME" default:"news" description:"RabbitMQ queue name"`
}

type DBConfig struct {
	Dsn          string `long:"db-dsn" env:"DB_DSN" description:"PostgreSQL DSN" default:"postgres://bbcrss:hunter2@mini/bbcrss?sslmode=disable"`
	MaxOpenConns int    `long:"db-max-open-conns" env:"DB_MAX_OPEN_CONNS" default:"25" description:"PostgreSQL max open connections"`
	MaxIdleConns int    `long:"db-max-idle-conns" env:"DB_MAX_IDLE_CONNS" default:"25" description:"PostgreSQL max idle connections"`
	MaxIdleTime  string `long:"db-max-idle-time" env:"DB_MAX_IDLE_TIME" default:"15m" description:"PostgreSQL max connection idle time"`
}

type APIConfig struct {
	Listen string `long:"listen" env:"LISTEN" default:":8080" description:"API server listen address"`
}

func main() {
	// Parsing cmd parameters
	var cfg Config
	p := flags.NewParser(&cfg, flags.PassDoubleDash|flags.HelpFlag)
	if _, err := p.Parse(); err != nil {
		if err.(*flags.Error).Type != flags.ErrHelp {
			fmt.Printf("%v\n", err)
			os.Exit(1)
		}
		p.WriteHelp(os.Stderr)
		os.Exit(2)
	}

	// Logger setup
	logOpts := []lgr.Option{
		lgr.LevelBraces,
		lgr.StackTraceOnError,
	}
	if cfg.Dbg {
		logOpts = append(logOpts, lgr.Debug)
	}
	lgr.SetupStdLogger(logOpts...)

	// Graceful termination
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		// catch signal and invoke graceful termination
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		<-stop
		log.Println("Shutdown signal received\n*********************************")
		cancel()
	}()

	defer func() {
		if x := recover(); x != nil {
			log.Printf("[WARN] run time panic: %+v", x)
		}
	}()

	// Service setup
	svc, err := NewService(&cfg)
	if err != nil {
		log.Fatalf("failed to start service: %v", err)
	}
	svc.Run(ctx)
}
