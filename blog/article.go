package blog

import "time"

type Article struct {
	UniqueID    string    `yaml:"uniqueid"`
	Slug       string    `yaml:"-"`
	Subject     string    `yaml:"subject"`
	Author      string    `yaml:"author"`
	AuthorHash  string    `yaml:"author_hash"`
	AuthorEmail string    `yaml:"author_email"`
	Date        time.Time `yaml:"date"`
	Banner      string    `yaml:"banner,omitempty"`
	Body        string    `yaml:"-"`
}

type Comment struct {
	UniqueID    string        `json:"uniqueid"`
	ParentID    string        `json:"-"`
	Author      string        `json:"author"`
	AuthorHash  string        `json:"author_hash"`
	AuthorEmail string        `json:"author_email"`
	Date        time.Time     `json:"date"`
	Body        string        `json:"body"`
	ReplyTo     string        `json:"reply_to"`
	Depth       int           `json:"-"`
	Deleted     bool          `json:"deleted,omitempty"`
	Edits       []CommentEdit `json:"edits,omitempty"`
}

type CommentEdit struct {
	Date time.Time `json:"date"`
	Body string    `json:"body"`
}

type DisplayComment struct {
	Comment    *Comment
	Depth      int
	ReplyTitle string
}
