# MailBlogger

用邮件写博客。发一封邮件即发布文章，回复邮件即发表评论。

## 快速开始

```bash
# 1. 复制并编辑配置文件
cp config.example.yaml config.yaml
# 填入邮件地址、IMAP/SMTP 凭证、白名单

# 2. 编译
go build -o mailblogger .

# 3. 启动
./mailblogger serve
# 打开 http://localhost:8080
```

## 接收邮件

**方式 A：IMAP 轮询** — 在 config.yaml 中配置 `mail.imap`。`./mailblogger serve` 每 30 秒拉取一次。`./mailblogger fetch` 单次拉取。

**方式 B：Cloudflare Email Worker**（推荐）— 无需 IMAP。Cloudflare 实时转发邮件到你的服务器。

```yaml
# config.yaml（仅 webhook，无 IMAP）
mail:
  address: blog@example.com
webhook:
  secret: "你的随机密钥"
```

部署 Worker（参见 `worker.example.js` + `wrangler.example.toml`），在 Cloudflare 控制台配置 Email Routing。

两种方式可同时启用。

## 发布文章

向博客地址发送邮件。发信人必须在白名单中。

```
From: 张三 <zhangsan@example.com>
To: blog@example.com
Subject: 我的第一篇文章

你好世界！支持 **Markdown** 格式。
```

## 发表评论

每篇文章和评论都有一个 8 位唯一 ID，显示在页面上。回复到 `blog+<ID>@域名`：

```
To: blog+afd888d6@example.com
Subject: Re: 我的第一篇文章

好文章！
```

## 编辑与删除

向 `blog+<文章ID>@域名` 发送邮件：

| 主题 | 效果 |
|---|---|
| `edit` | 替换正文。启用历史记录时旧版本归档到 `edit_N/` |
| `delete` | 移至 `_deleted/` 或永久删除 |

评论同理：向 `blog+<评论ID>@域名` 发送 `edit` 或 `delete`。

## 正文配置

在邮件正文开头声明选项：

```
---
banner: 2
slug: my-post
notify: on
title: 自定义标题
---

文章正文从这里开始。
```

| 键 | 说明 |
|---|---|
| `banner` | 用作页面横幅的图片编号（替换站点头像） |
| `slug` | 自定义 URL 路径（小写字母、数字、连字符） |
| `title` | 覆盖文章标题 |
| `notify` | `on`/`true` → 关注；`off`/`false` → 静音 |

## 图片引用

在 Markdown 中用序号引用图片：`![描述](1)` → 保存为 `![描述](1.webp)`。

## 通知

有人回复你的评论时，你会收到通知邮件。直接点回复即可继续讨论——`Reply-To` 头会将邮件路由回讨论线程。

发送主题为 `settings` 的邮件可配置通知偏好。

三级优先级：单篇文章覆盖 > 个人偏好 > 全局默认。

## API

只读 JSON API，可用于构建自定义前端。详见 [docs/api.md](docs/api.md)。

| 端点 | 说明 |
|---|---|
| `GET /api/site` | 站点信息 |
| `GET /api/articles` | 所有文章 |
| `GET /api/article/{id}` | 文章详情（按 hash 或 slug） |
| `GET /api/article/{id}/comments` | 文章评论 |
| `POST /api/article` | 创建文章 |
| `POST /api/comment` | 创建评论 |
| `POST /api/raw-email` | Webhook：接收原始邮件 |

## 主题

主题控制整个前端。在 config.yaml 中设置：

```yaml
theme: default
```

主题文件放在 `themes/<名称>/` 下。完整的主题开发指南见 [docs/themes.md](docs/themes.md)。

文章的 `body_html` 由后端渲染，包含 GFM、图片 figure，以及围栏代码块的复制按钮标记。主题可直接样式化这套共享输出，并自行绑定复制交互。

## 配置

所有选项见 `config.example.yaml`。主要字段：

```yaml
mail:
  address: blog@example.com
  imap:
    server: imap.example.com
    username: blog@example.com
    password: your-password
  smtp:
    server: smtp.example.com
    port: 465
  whitelist:
    - "*@example.com"

site:
  lang: zh
  show_author: true
  width: 600

web:
  port: 8080

privacy:
  hide_email: true

history:
  article:
    keep: true
  comment:
    keep: true
  show_deleted: true
```

所有配置项支持热修改——编辑 `config.yaml` 后立即生效。

## 内容结构

```
content/
├── 20260713_afd888d6_hello-world/
│   ├── index.md           # frontmatter + markdown 正文
│   ├── comments.json      # 评论 JSON 数组
│   ├── 1.webp             # 文章图片
│   └── edit_0/            # 归档版本（启用历史记录时）
├── _drafts/               # 非白名单投稿
├── _deleted/              # 已删除文章归档
└── mailblogger.db         # SQLite 元数据
```

## 隐私

- 作者名称取自邮件 `From:` 头；无名称时显示哈希
- 作者邮箱默认隐藏（`privacy.hide_email`）
- 访客通过 `blog+<author_hash>@域名` 联系作者，无需知道真实邮箱
- 通知的 `Reply-To` 经由博客服务器路由，不暴露回复者邮箱

## Docker

```bash
docker build -t mailblogger .
docker run -p 8080:8080 \
  -v ./config.yaml:/app/config.yaml \
  -v ./content:/app/content \
  mailblogger
```

## 许可证

MIT