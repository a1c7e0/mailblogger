package main

import (
	"crypto/tls"
	"encoding/base64"
	"flag"
	"fmt"
	"net/smtp"
	"os"
	"time"
)

func rfc2047(s string) string {
	needs := false
	for i := 0; i < len(s); i++ {
		if s[i] > 127 {
			needs = true
			break
		}
	}
	if !needs {
		return s
	}
	return "=?utf-8?B?" + base64.StdEncoding.EncodeToString([]byte(s)) + "?="
}

func main() {
	fromAddr := flag.String("from", "tester@owowo.dev", "from address")
	fromName := flag.String("name", "", "from display name")
	flag.Parse()

	args := flag.Args()
	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: sendmail [-from addr] [-name name] <to> <subject> [body]\n")
		os.Exit(1)
	}

	user := os.Getenv("SMTP_USER")
	pass := os.Getenv("SMTP_PASS")
	host := os.Getenv("SMTP_HOST")
	if user == "" || pass == "" || host == "" {
		fmt.Fprintf(os.Stderr, "SMTP_USER, SMTP_PASS, and SMTP_HOST environment variables are required\n")
		os.Exit(1)
	}
	to := args[0]
	subject := args[1]
	body := ""
	if len(args) > 2 {
		body = args[2]
	} else {
		data, _ := os.ReadFile("/dev/stdin")
		body = string(data)
	}

	fromHeader := rfc2047(*fromAddr)
	if *fromName != "" {
		fromHeader = rfc2047(*fromName) + " <" + *fromAddr + ">"
	}

	msgID := fmt.Sprintf("<test-%d@mailblogger>", time.Now().UnixNano())
	msg := "From: " + fromHeader + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + rfc2047(subject) + "\r\n" +
		"Date: " + time.Now().Format(time.RFC1123Z) + "\r\n" +
		"Message-ID: " + msgID + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		body

	addr := host + ":465"
	tlsCfg := &tls.Config{ServerName: host}
	conn, err := tls.Dial("tcp", addr, tlsCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tls dial: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	c, err := smtp.NewClient(conn, host)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smtp client: %v\n", err)
		os.Exit(1)
	}
	defer c.Quit()

	auth := smtp.PlainAuth("", user, pass, host)
	if err := c.Auth(auth); err != nil {
		fmt.Fprintf(os.Stderr, "auth: %v\n", err)
		os.Exit(1)
	}
	if err := c.Mail(user); err != nil {
		fmt.Fprintf(os.Stderr, "mail from: %v\n", err)
		os.Exit(1)
	}
	if err := c.Rcpt(to); err != nil {
		fmt.Fprintf(os.Stderr, "rcpt to: %v\n", err)
		os.Exit(1)
	}
	w, err := c.Data()
	if err != nil {
		fmt.Fprintf(os.Stderr, "data: %v\n", err)
		os.Exit(1)
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		fmt.Fprintf(os.Stderr, "write: %v\n", err)
		os.Exit(1)
	}
	if err := w.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "close: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("sent OK  -> %s\n", to)
}
