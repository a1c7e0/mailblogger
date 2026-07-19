# MailBlogger

用邮件写博客。发一封邮件即发布文章，回复邮件即发表评论。

页面风格参考内核邮件列表（kernel mailing list）—— 等宽字体、左对齐、零 JS 要求（优雅降级）。

## 快速开始

```bash
# 1. 编辑 config.yaml，填入你的 IMAP/SMTP 凭证
# 2. 编译
go build -o mailblogger .

# 3. 一次性拉取（检查收件箱，处理新邮件）
./mailblogger fetch

# 4. 启动 Web 服务 + 每 60 秒自动拉取
./mailblogger serve

# 打开 http://localhost:8080
```

## 工作原理

### 发布文章

向 `wmail@owowo.dev`（或你配置的任意地址）发送邮件。发信人必须在白名单中（`config.yaml`）。

```
From: 张三 <zhangsan@example.com>
To: wmail@owowo.dev
Subject: 我的第一篇文章

你好世界！这是 Markdown 格式的正文。
```

### 发表评论

每篇文章都有一个 8 位唯一 ID，显示在页面上。回复到 `wmail+<唯一ID>@owowo.dev`：

```
From: 读者 <reader@example.com>
To: wmail+afd888d6@owowo.dev
Subject: Re: 我的第一篇文章

好文章！我有个问题……
```

### 回复某条评论

每条评论也有自己的唯一 ID。发送到 `wmail+<评论ID>@owowo.dev`：

```
From: 另一个读者 <other@example.com>
To: wmail+92d93709@owowo.dev
Subject: Re: 我的第一篇文章

我也想知道答案。
```

### 邮件通知

有人回复你的评论时，你会收到一封通知邮件。通知邮件的 `Reply-To` 头指向 `wmail+<新评论ID>@owowo.dev` —— 在邮件客户端直接点回复就能继续参与讨论。

### 编辑 / 删除文章

作为文章作者，向 `wmail+<文章ID>@owowo.dev` 发送邮件：

| 操作 | 主题 | 效果 |
|---|---|---|
| 编辑 | `[EDIT] 新标题` | 替换文章标题和正文 |
| 删除 | `[DELETE]` | 删除整个文章目录 |

## 配置说明

```yaml
# config.yaml
imap:
  server: imap.purelymail.com
  port: 993
  username: wmail@owowo.dev
  password: xvgkbjtnztexspcbjsuz

smtp:
  server: smtp.purelymail.com
  port: 465
  # 用户名和密码不填则自动使用 IMAP 凭证

domain: owowo.dev          # 邮件域名
content_dir: content       # 文章存储目录

whitelist:                 # 允许发布文章的发信人
  - "*@owowo.dev"

site:
  title: 我的博客           # 页面标题
  subtitle: ""             # 可选副标题
  footer_html: ""          # 页脚 HTML（支持 <a>、<script> 等）

web:
  port: 8080
  host: 0.0.0.0
```

## 内容结构

```
content/
└── afd888d6/              # 文章目录（以唯一 ID 命名）
    ├── index.md           # 文章（YAML frontmatter + Markdown 正文）
    └── comments.md        # 评论（多个 YAML 文档块）
```

## 隐私设计

- 作者名称取自邮件 From 头的显示名（如 `张三` 来自 `张三 <a@b.com>`）
- 未设置名称时，显示邮箱的 SHA256 哈希（前 8 位）
- 作者邮箱地址 **绝不会** 出现在 HTML 输出中
- 每位作者通过稳定的 `author_hash` 标识（邮箱 SHA256 前 8 位）
- 所有文章和评论均显示唯一 ID

## 测试

项目附带一个 SMTP 发送工具用于开发调试：

```bash
cd tools && go build -o sendmail sendmail.go

# 发送测试文章
./sendmail "wmail@owowo.dev" "你好世界" "# Markdown 正文"

# 发送测试评论（将 <uid> 替换为实际 ID）
./sendmail -name "张三" -from "zhangsan@example.com" "wmail+<uid>@owowo.dev" "Re: 你好" "好文章！"

# 处理邮件
cd .. && ./mailblogger fetch
```
