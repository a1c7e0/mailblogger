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
	store.History = blog.HistoryConfig{
		ArticleKeep:    cfg.History.Article.Keep,
		ArticleVisible: cfg.History.Article.Visible,
		CommentKeep:    cfg.History.Comment.Keep,
		CommentVisible: cfg.History.Comment.Visible,
		ShowDeleted:    cfg.History.ShowDeleted,
		ShowReplies:    cfg.History.ShowReplies,
	}

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

	sender := &email.SMTPSender{}
	if cfg.Mail.SMTP.Server != "" && cfg.Mail.SMTP.Password != "" {
		sender = email.NewSMTPSender(cfg.Mail.SMTP.Server, cfg.Mail.SMTP.Port, cfg.Mail.SMTP.Username, cfg.Mail.SMTP.Password)
	}

	c, err := email.ConnectIMAP(imapCfg)
	if err != nil {
		log.Fatalf("connect imap: %v", err)
	}
	defer c.Logout()

	messages, err := email.FetchUnseen(c)
	if err != nil {
		log.Fatalf("fetch unseen: %v", err)
	}

	if len(messages) == 0 {
		log.Println("no new messages")
		return
	}

	processor := email.NewProcessor(store, cfg.EmailLocal, cfg.EmailDomain, cfg.Host, cfg.Web.Scheme, cfg.Mail.Whitelist, sender)
	var processedSeqs []uint32

	for _, msg := range messages {
		if err := processor.ProcessMessage(msg); err != nil {
			log.Printf("error processing message UID=%d: %v", msg.SeqNum, err)
			continue
		}
		processedSeqs = append(processedSeqs, msg.SeqNum)
	}

	if len(processedSeqs) > 0 {
		if err := email.DeleteEmails(c, processedSeqs); err != nil {
			log.Printf("error deleting processed emails: %v", err)
		} else {
			log.Printf("deleted %d processed emails", len(processedSeqs))
		}
	}
}

func runServe(cfg *config.Config, store *blog.Store, configPath string) {
	srv, err := web.NewServer(store, cfg.Host, cfg.Web.Scheme, cfg.EmailLocal, cfg.EmailDomain, cfg.Privacy.HideEmail, cfg.Site, cfg.Web.Host, cfg.Web.Port)
	if err != nil {
		log.Fatalf("init server: %v", err)
	}

	store.SetOnChange(func() {
		srv.InvalidateFeedCache()
	})

	srv.SetConfigGetter(func() *config.Config {
		return currentConfig.Load()
	})

	sender := &email.SMTPSender{}
	if cfg.Mail.SMTP.Server != "" && cfg.Mail.SMTP.Password != "" {
		sender = email.NewSMTPSender(cfg.Mail.SMTP.Server, cfg.Mail.SMTP.Port, cfg.Mail.SMTP.Username, cfg.Mail.SMTP.Password)
	}

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
		go imapPoller(store, sender, done)
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

func imapPoller(store *blog.Store, sender *email.SMTPSender, done <-chan struct{}) {
	pollInterval := 30 * time.Second
	backoff := 1 * time.Second
	maxBackoff := 2 * time.Minute

	for {
		select {
		case <-done:
			return
		default:
		}

		cfg := currentConfig.Load()
		processor := email.NewProcessor(store, cfg.EmailLocal, cfg.EmailDomain, cfg.Host, cfg.Web.Scheme, cfg.Mail.Whitelist, sender)

		imapCfg := email.Config{
			Server:   cfg.Mail.IMAP.Server,
			Port:     cfg.Mail.IMAP.Port,
			Username: cfg.Mail.IMAP.Username,
			Password: cfg.Mail.IMAP.Password,
		}

		err := imapFetchAndProcess(imapCfg, processor)
		if err != nil {
			log.Printf("imap: %v (retry in %v)", err, backoff)
			select {
			case <-time.After(backoff):
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
				continue
			case <-done:
				return
			}
		}

		backoff = 1 * time.Second

		select {
		case <-time.After(pollInterval):
		case <-done:
			return
		}
	}
}

func imapFetchAndProcess(imapCfg email.Config, processor *email.Processor) error {
	c, err := email.ConnectIMAP(imapCfg)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer c.Logout()

	messages, err := email.FetchUnseen(c)
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}

	if len(messages) == 0 {
		return nil
	}

	var seqs []uint32
	for _, msg := range messages {
		if err := processor.ProcessMessage(msg); err != nil {
			log.Printf("process UID=%d: %v", msg.SeqNum, err)
			continue
		}
		seqs = append(seqs, msg.SeqNum)
	}

	if len(seqs) > 0 {
		if err := email.DeleteEmails(c, seqs); err != nil {
			log.Printf("delete: %v", err)
		}
	}

	return nil
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
