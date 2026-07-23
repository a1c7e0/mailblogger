package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Mail       MailConfig    `yaml:"mail"`
	ContentDir string        `yaml:"content_dir"`
	Site       SiteConfig    `yaml:"site"`
	Web        WebConfig     `yaml:"web"`
	Privacy    PrivacyConfig `yaml:"privacy"`
	Webhook    WebhookConfig `yaml:"webhook"`
	History    HistoryConfig `yaml:"history"`
	Theme      ThemeConfig   `yaml:"theme"`

	Host string `yaml:"-"`

	EmailLocal  string `yaml:"-"`
	EmailDomain string `yaml:"-"`
}

type MailConfig struct {
	Address   string       `yaml:"address"`
	IMAP      IMAPConfig   `yaml:"imap"`
	SMTP      SMTPConfig   `yaml:"smtp"`
	Whitelist []string     `yaml:"whitelist"`
	Notify    NotifyConfig `yaml:"notify"`
	DKIM      DKIMPolicy   `yaml:"dkim"`
}

type NotifyConfig struct {
	Article bool `yaml:"article"`
	Comment bool `yaml:"comment"`
}

// DKIMPolicy controls DKIM verification strictness.
// "none" = skip DKIM entirely
// "normal" = reject invalid signatures, allow unsigned (default)
// "strict" = reject invalid + unsigned for comments
type DKIMPolicy string

const (
	DKIMNone   DKIMPolicy = "none"
	DKIMNormal DKIMPolicy = "normal"
	DKIMStrict DKIMPolicy = "strict"
)

type IMAPConfig struct {
	Server   string `yaml:"server"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type SMTPConfig struct {
	Server   string `yaml:"server"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type SiteConfig struct {
	Lang        string    `yaml:"lang"`
	ShowAuthor  bool      `yaml:"show_author"`
	Avatar      string    `yaml:"-"`
	Width       int       `yaml:"width"`
	Links       []NavLink `yaml:"links"`
	AutoLang    bool      `yaml:"auto_lang"`
}

type NavLink struct {
	Title string `yaml:"title"`
	URL   string `yaml:"url"`
}

type WebConfig struct {
	Port   int    `yaml:"port"`
	Host   string `yaml:"host"`
	Scheme string `yaml:"scheme"`
}

type PrivacyConfig struct {
	HideEmail bool `yaml:"hide_email"`
}

type WebhookConfig struct {
	Secret string `yaml:"secret"`
}

// ThemeConfig handles both string and map YAML for theme field
type ThemeConfig struct {
	// Single theme mode: Theme is set
	Theme string
	// Per-language mode: Themes map is set
	Themes map[string]string
}

func (t *ThemeConfig) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		t.Theme = value.Value
	case yaml.MappingNode:
		t.Themes = make(map[string]string)
		for i := 0; i < len(value.Content)-1; i += 2 {
			t.Themes[value.Content[i].Value] = value.Content[i+1].Value
		}
	default:
		return fmt.Errorf("theme must be a string or map")
	}
	return nil
}

// ResolveTheme returns the theme directory name for the given language
func (t *ThemeConfig) ResolveTheme(lang string) string {
	if len(t.Themes) > 0 {
		if theme, ok := t.Themes[lang]; ok {
			return theme
		}
		// Fallback to first theme
		for _, theme := range t.Themes {
			return theme
		}
	}
	return t.Theme
}

type HistoryConfig struct {
	Article HistoryToggle `yaml:"article"`
	Comment HistoryToggle `yaml:"comment"`
	ShowDeleted bool     `yaml:"show_deleted"`
	ShowReplies bool     `yaml:"show_replies"`
}

type HistoryToggle struct {
	Keep    bool `yaml:"keep"`
	Visible bool `yaml:"visible"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := &Config{
		Site: SiteConfig{ShowAuthor: true, Lang: "en", Width: 600},
		History: HistoryConfig{
			Article:     HistoryToggle{Keep: true, Visible: true},
			Comment:     HistoryToggle{Keep: true, Visible: true},
			ShowDeleted: true,
			ShowReplies: true,
		},
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	if cfg.ContentDir == "" {
		cfg.ContentDir = "content"
	}
	if cfg.Mail.Address != "" {
		parts := strings.SplitN(cfg.Mail.Address, "@", 2)
		if len(parts) == 2 {
			cfg.EmailLocal = parts[0]
			cfg.EmailDomain = parts[1]
		}
	}
	if cfg.Host == "" {
		cfg.Host = cfg.EmailDomain
	}
	if cfg.Site.Width == 0 {
		cfg.Site.Width = 600
	}
	if cfg.Web.Port == 0 {
		cfg.Web.Port = 8080
	}
	if cfg.Web.Host == "" {
		cfg.Web.Host = "0.0.0.0"
	}
	if cfg.Web.Scheme == "" {
		cfg.Web.Scheme = "https"
	}
	if cfg.Mail.IMAP.Port == 0 {
		cfg.Mail.IMAP.Port = 993
	}
	if cfg.Mail.SMTP.Port == 0 {
		cfg.Mail.SMTP.Port = 465
	}
	if cfg.Mail.SMTP.Username == "" {
		cfg.Mail.SMTP.Username = cfg.Mail.IMAP.Username
	}
	if cfg.Mail.SMTP.Password == "" {
		cfg.Mail.SMTP.Password = cfg.Mail.IMAP.Password
	}
	return cfg, nil
}
