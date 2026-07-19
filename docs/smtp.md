# SMTP Sender

## File: `email/smtp.go`

Used for reply notifications. Not used for sending articles/comments (those come via IMAP).

## Connection
- Implicit TLS on port 465 (`crypto/tls` + `net/smtp`)
- PLAIN authentication
- Single-message send: `Mail()` → `Rcpt()` → `Data()` → `Write()` → `Close()`

## SMTPSender Struct
```go
type SMTPSender struct {
    Server   string
    Port     int
    Username string
    Password string
}
```

## Usage
```go
sender := email.NewSMTPSender("smtp.purelymail.com", 465, "user", "pass")
sender.Send("from@domain", "to@domain", rawEmailData)
```

## Notification Email Format
```
From: <mailbox>@<domain>
To: <parent_comment_author_email>
Subject: Re: <article_subject>
Reply-To: <mailbox>+<new_comment_uid>@<domain>
MIME-Version: 1.0
Content-Type: text/plain; charset=utf-8

<reply_author> (#<reply_uid>) wrote:
> <reply_body>

In reply to <parent_author> (#<parent_uid>):
> <parent_body>

In reply to <grandparent_author> (#<grandparent_uid>):
> <grandparent_body>

---
Reply to this email to respond directly.

The email includes the full discussion thread (up to 5 ancestors) so the recipient can follow context. Each message is labeled with `Name (#uniqueid)`. Thread content truncates at 6000 characters.

The `Reply-To` header is the key design element: when the recipient hits "Reply" in their email client, the message goes to `<mailbox>+<new_comment_uid>@<domain>`, which routes it back to the comment thread.

## Test Tool
`tools/sendmail.go` — standalone SMTP sender for development:
```bash
cd tools && go build -o sendmail sendmail.go
./sendmail -name "Alice" -from "a@b.com" "to@domain" "subject" "body"
```
