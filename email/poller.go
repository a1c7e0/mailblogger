package email

import (
	"fmt"
	"log"
	"time"

	"mailblogger/blog"
	"mailblogger/config"
)

// FetchOnce connects to IMAP, fetches unseen messages, processes them, and deletes processed emails.
func FetchOnce(imapCfg Config, processor *Processor) error {
	c, err := ConnectIMAP(imapCfg)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer c.Logout()

	messages, err := FetchUnseen(c)
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
		if err := DeleteEmails(c, seqs); err != nil {
			log.Printf("delete: %v", err)
		}
	}

	return nil
}

// Poller periodically fetches emails from IMAP and processes them.
type Poller struct {
	Store        *blog.Store
	ConfigGetter func() *config.Config
	Done         <-chan struct{}
}

// NewPoller creates a new IMAP poller.
func NewPoller(store *blog.Store, configGetter func() *config.Config, done <-chan struct{}) *Poller {
	return &Poller{
		Store:        store,
		ConfigGetter: configGetter,
		Done:         done,
	}
}

// Start begins the polling loop. It blocks until Done is closed.
func (p *Poller) Start() {
	pollInterval := 30 * time.Second
	backoff := 1 * time.Second
	maxBackoff := 2 * time.Minute

	for {
		select {
		case <-p.Done:
			return
		default:
		}

		cfg := p.ConfigGetter()
		sender := NewSenderFromConfig(cfg.Mail.SMTP)
		processor := NewProcessor(p.Store, cfg.EmailLocal, cfg.EmailDomain, cfg.Host, cfg.Web.Scheme, cfg.Mail.Whitelist, sender, cfg.Mail.DKIM)

		imapCfg := Config{
			Server:   cfg.Mail.IMAP.Server,
			Port:     cfg.Mail.IMAP.Port,
			Username: cfg.Mail.IMAP.Username,
			Password: cfg.Mail.IMAP.Password,
		}

		err := FetchOnce(imapCfg, processor)
		if err != nil {
			log.Printf("imap: %v (retry in %v)", err, backoff)
			select {
			case <-time.After(backoff):
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
				continue
			case <-p.Done:
				return
			}
		}

		backoff = 1 * time.Second

		select {
		case <-time.After(pollInterval):
		case <-p.Done:
			return
		}
	}
}
