package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"

	"mailblogger/blog"
	"mailblogger/config"
	"mailblogger/email"
	"mailblogger/web"

	"github.com/fsnotify/fsnotify"
)

var currentConfig atomic.Pointer[config.Config]

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		args = []string{"serve"}
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	currentConfig.Store(cfg)

	absContentDir, err := filepath.Abs(cfg.ContentDir)
	if err != nil {
		log.Fatalf("resolve content dir: %v", err)
	}

	store, err := blog.NewStore(absContentDir)
	if err != nil {
		log.Fatalf("init store: %v", err)
	}
	store.SetDefaultNotify(cfg.Mail.Notify.Article, cfg.Mail.Notify.Comment)
	store.History = cfg.History

	switch args[0] {
	case "fetch":
		runFetch(cfg, store)
	case "serve":
		runServe(cfg, store, *configPath)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		fmt.Fprintf(os.Stderr, "usage: mailblogger [fetch|serve]\n")
		os.Exit(1)
	}
}

func runFetch(cfg *config.Config, store *blog.Store) {
	if cfg.Mail.IMAP.Server == "" {
		log.Println("IMAP not configured, nothing to fetch")
		return
	}

	imapCfg := email.Config{
		Server:   cfg.Mail.IMAP.Server,
		Port:     cfg.Mail.IMAP.Port,
		Username: cfg.Mail.IMAP.Username,
		Password: cfg.Mail.IMAP.Password,
	}

	sender := email.NewSenderFromConfig(cfg.Mail.SMTP)

	processor := email.NewProcessor(store, cfg.EmailLocal, cfg.EmailDomain, cfg.Host, cfg.Web.Scheme, cfg.Mail.Whitelist, sender, cfg.Mail.DKIM)

	if err := email.FetchOnce(imapCfg, processor); err != nil {
		log.Fatalf("fetch: %v", err)
	}
}

func runServe(cfg *config.Config, store *blog.Store, configPath string) {
	srv, err := web.NewServer(web.ServerConfig{
		Store:       store,
		Host:        cfg.Host,
		Scheme:      cfg.Web.Scheme,
		EmailLocal:  cfg.EmailLocal,
		EmailDomain: cfg.EmailDomain,
		Site:        cfg.Site,
		Theme:       cfg.Theme,
	})
	if err != nil {
		log.Fatalf("init server: %v", err)
	}

	store.SetOnChange(func() {
		srv.InvalidateFeedCache()
	})

	srv.SetConfigGetter(func() *config.Config {
		return currentConfig.Load()
	})

	addr := fmt.Sprintf("%s:%d", cfg.Web.Host, cfg.Web.Port)
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Printf("Web server listening on http://%s", addr)
		if err := httpSrv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	done := make(chan struct{})
	if cfg.Mail.IMAP.Server != "" {
		poller := email.NewPoller(store, func() *config.Config { return currentConfig.Load() }, done)
		go poller.Start()
		log.Println("IMAP poller started")
	} else {
		log.Println("IMAP not configured, webhook-only mode")
	}

	go watchConfig(configPath, srv)

	select {
	case <-quit:
		log.Println("shutting down...")
		close(done)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		httpSrv.Shutdown(ctx)
	}
}

func watchConfig(configPath string, srv *web.Server) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("config watch: %v", err)
		return
	}
	defer watcher.Close()

	if err := watcher.Add(configPath); err != nil {
		log.Printf("config watch add: %v", err)
		return
	}

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				newCfg, err := config.Load(configPath)
				if err == nil {
					currentConfig.Store(newCfg)
					srv.UpdateConfig(newCfg.Site)
					log.Println("config reloaded")
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("config watch error: %v", err)
		}
	}
}
