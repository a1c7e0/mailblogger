# SMTP Sender

## File: `email/smtp.go`

Used for reply notifications and error replies only. Articles/comments arrive via IMAP or webhook.

## Connection

- Implicit TLS on port 465 (`crypto/tls` + `net/smtp`)
- PLAIN authentication
- `NewSMTPSender(server, port, username, password)`
- `NewSenderFromConfig(smtpCfg)` — factory that returns no-op sender when SMTP not configured
- `Send(from, to, rawEmailData)` — single-message send

## Notification Email Format

```
From: <mailbox>@<domain>
To: <parent_author_email>
Subject: Re: <article_subject>
Reply-To: <mailbox>+<new_comment_uid>@<domain>
Content-Type: text/plain; charset=utf-8

<reply_author> (#<reply_uid>) wrote:
> <reply_body>

In reply to <parent_author> (#<parent_uid>):
> <parent_body>

---
Reply to this email to respond directly.
```

Thread context: up to 4 ancestors (configurable), 6000 char limit. Each labeled `Name (#uniqueid)`.

`Reply-To` is the key: recipient's "Reply" routes to `<mailbox>+<new_comment_uid>@domain`.

## Test Tool

```bash
cd tools && go build -o sendmail sendmail.go
./sendmail -name "Alice" -from "a@b.com" "to@domain" "subject" "body"
```