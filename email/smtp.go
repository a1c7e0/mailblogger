package email

import (
	"crypto/tls"
	"fmt"
	"net/smtp"
)

type SMTPSender struct {
	Server   string
	Port     int
	Username string
	Password string
}

func NewSMTPSender(server string, port int, username, password string) *SMTPSender {
	return &SMTPSender{
		Server:   server,
		Port:     port,
		Username: username,
		Password: password,
	}
}

func (s *SMTPSender) Send(from, to, data string) error {
	addr := fmt.Sprintf("%s:%d", s.Server, s.Port)

	tlsCfg := &tls.Config{ServerName: s.Server}
	conn, err := tls.Dial("tcp", addr, tlsCfg)
	if err != nil {
		return fmt.Errorf("tls dial: %w", err)
	}
	defer conn.Close()

	c, err := smtp.NewClient(conn, s.Server)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer c.Quit()

	auth := smtp.PlainAuth("", s.Username, s.Password, s.Server)
	if err := c.Auth(auth); err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("rcpt to: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}
	if _, err := w.Write([]byte(data)); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return w.Close()
}
