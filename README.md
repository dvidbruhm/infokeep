# InfoKeep

A self-hosted personal information manager for bookmarks, notes, recipes, drawings, rated lists, checklists, and media. Built with Go and SQLite — fast, lightweight, and easy to run anywhere.

![InfoKeep Dashboard](web/static/screenshots/dashboard.png)

---

## ✨ Features

| Category | Details |
|---|---|
| 🔖 **Bookmarks** | Save URLs with title, description, tags, and auto-fetched favicons |
| 📝 **Notes** | Rich text notes with tagging |
| 🍳 **Recipes** | Save and organize recipes with ingredients, instructions, images, and source URL. Includes an automatic recipe parser |
| ⭐ **Rated Lists** | Create lists (movies, books, games…) and score each entry out of 10 |
| ✅ **Checklists** | To-do and checklist tracking |
| 🖼️ **Media** | Upload and manage images |
| 🎨 **Drawings** | Built-in canvas for freehand drawings |
| 🔍 **Search** | Fast full-text search across all categories |
| 🏷️ **Tags** | Tag anything, filter by tag from the sidebar |
| 🎨 **Themes** | Light, Dark, Sepia, Dracula, Catppuccin |
| 🔐 **Multi-user** | Registration, session-based login, per-user data isolation |
| 🦊 **Firefox Extension** | Clip bookmarks, notes, recipes, and rated list items directly from your browser |

---

## 🚀 Getting Started

### Option 1 — Docker Compose (recommended)

```bash
git clone https://github.com/dvidbruhm/infokeep.git
cd infokeep
docker compose up -d
```

The app will be available at **http://localhost:8989**.

> Data is persisted via Docker volumes — your database and uploads survive container restarts.

### Option 2 — Build from source

**Requirements:** Go 1.24+, GCC (for CGO / SQLite)

```bash
git clone https://github.com/dvidbruhm/infokeep.git
cd infokeep
go build -o infokeep .
./infokeep
```

The app starts on **http://localhost:8080**.

---

## 🛠️ Tech Stack

| Layer | Technology |
|---|---|
| Backend | [Go](https://golang.org/) + [chi](https://github.com/go-chi/chi) router |
| Database | [SQLite](https://sqlite.org/) via [go-sqlite3](https://github.com/mattn/go-sqlite3) |
| Frontend | [Bulma CSS](https://bulma.io/) + [HTMX](https://htmx.org/) |
| Auth | Session cookies + bcrypt password hashing |
| Templating | Go `html/template` |
| Container | Docker + Docker Compose |

---

## 🦊 Firefox Extension

The InfoKeep Clipper extension lets you save content from any page without leaving your browser.

### Setup

1. Go to **Settings → Browser Extension API Token** in InfoKeep and click **Copy**.
2. Install the extension (load it temporarily via `about:debugging` or install the `.zip`).
3. Click the extension icon → **⚙ Settings** tab → paste your token → **Save Token**.

### What you can clip

- **Bookmarks** — title, URL, description, tags
- **Notes** — title, content, tags
- **Recipes** — auto-parsed from the current page URL
- **Rated List Items** — add to any existing rated list with a score

> The extension uses a Bearer API token for authentication, so you do **not** need to be logged into InfoKeep in the same browser tab.

---

## 📁 Project Structure

```
infokeep/
├── main.go                     # Server entry point + routes
├── internal/
│   ├── database/db.go          # SQLite schema, migrations, queries
│   └── handlers/
│       ├── handlers.go         # All HTTP handlers + middleware
│       └── recipe_parser.go    # Automatic recipe web scraper
├── web/
│   ├── templates/              # Go HTML templates + layout
│   └── static/                 # CSS, JS, icons, uploads
├── firefox-extension/          # Browser extension source
│   ├── manifest.json
│   ├── popup.html
│   ├── popup.js
│   └── popup.css
├── Dockerfile
└── docker-compose.yml
```

---

## ⚙️ Configuration

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | Port the server listens on |

The database file (`infokeep.db`) is created automatically in the working directory on first run.

---

## 🔒 Security Notes

- Passwords are hashed with **bcrypt**.
- Sessions are stored server-side in SQLite with expiry.
- API tokens are random 64-character hex strings.
- All data is scoped per user — users cannot access each other's data.

---

## 📦 Building the Firefox Extension

```bash
cd firefox-extension
zip -r ../firefox-extension.zip .
```

Or on Windows (PowerShell):

```powershell
Compress-Archive -Path firefox-extension\* -DestinationPath firefox-extension.zip -Force
```

Submit `firefox-extension.zip` to [addons.mozilla.org](https://addons.mozilla.org).

---
