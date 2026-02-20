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

### Option 1 — Run locally (development)

**Requirements:** Go 1.24+, GCC (for CGO / SQLite)

```bash
git clone https://github.com/dvidbruhm/infokeep.git
cd infokeep
go build -o infokeep .
./infokeep
```

The app starts on **http://localhost:8080**.

### Option 2 — Docker Compose

```bash
git clone https://github.com/dvidbruhm/infokeep.git
cd infokeep
docker compose up -d
```

The app will be available at **http://localhost:8989**.

> Data is persisted via Docker volumes — your database and uploads survive container restarts.

---

## 🏠 Self-Hosting on Windows with Cloudflare Tunnel

Run InfoKeep on your own PC and make it accessible from anywhere on the internet — for free, with automatic HTTPS, and without exposing your home IP.

### Prerequisites

- A domain name pointed to Cloudflare (see [Domain Setup](#domain-setup) below)
- A free [Cloudflare account](https://cloudflare.com)

---

### Step 1 — Run the app

```powershell
# In the infokeep project folder:
.\infokeep.exe
# or with Docker:
docker compose up -d
```

The app runs on `http://localhost:8080` (or `:8989` with Docker). Keep this running.

---

### Step 2 — Install cloudflared

Download `cloudflared-windows-amd64.exe` from the [Cloudflare downloads page](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/) and save it somewhere permanent, e.g. `C:\cloudflared\cloudflared.exe`.

---

### Step 3 — Create the tunnel

Run these commands in PowerShell:

```powershell
# Log in to Cloudflare (opens your browser)
C:\cloudflared\cloudflared.exe tunnel login

# Create a named tunnel
C:\cloudflared\cloudflared.exe tunnel create infokeep

# Point your domain at the tunnel
# (delete any conflicting A/AAAA/CNAME records in Cloudflare DNS first)
C:\cloudflared\cloudflared.exe tunnel route dns infokeep yourdomain.com
```

> **Conflicting record error?** Go to Cloudflare Dashboard → your domain → **DNS → Records** and delete any existing `A`, `AAAA`, or `CNAME` record for the root (`@`) hostname, then run the last command again.

---

### Step 4 — Create the config file

Create `C:\cloudflared\config.yml`:

```yaml
tunnel: infokeep
credentials-file: C:\Users\YourName\.cloudflared\<tunnel-id>.json

ingress:
  - hostname: yourdomain.com
    service: http://localhost:8080
  - service: http_status:404
```

Replace `<tunnel-id>` with the UUID shown when you ran `tunnel create` (also visible as the filename in `C:\Users\YourName\.cloudflared\`).

---

### Step 5 — Run the tunnel

```powershell
C:\cloudflared\cloudflared.exe tunnel run infokeep
```

Your app is now live at `https://yourdomain.com` with automatic HTTPS. ✅

---

### Step 6 — Auto-start on boot (optional)

Install cloudflared as a Windows service so it starts automatically:

```powershell
C:\cloudflared\cloudflared.exe service install
```

To also auto-start InfoKeep, add a shortcut to `infokeep.exe` to your Startup folder:
- Press `Win+R` → type `shell:startup` → paste a shortcut to `infokeep.exe` there

---

### Step 7 — Update the Firefox Extension

Edit `firefox-extension/popup.js` line 1:

```js
const API_BASE = "https://yourdomain.com/api";
```

Then repackage and reload the extension.

---

### Domain Setup

You need a domain with its nameservers pointed to Cloudflare.

**Cheapest path:**
1. Buy a `.xyz` domain for ~$1–2/yr on [Namecheap](https://namecheap.com) or [Porkbun](https://porkbun.com)
2. In Cloudflare → **Add a site** → enter your domain → choose the **Free plan**
3. Cloudflare gives you two nameserver addresses (e.g. `aria.ns.cloudflare.com`)
4. Go back to Namecheap → your domain → **Nameservers** → **Custom DNS** → paste the two Cloudflare nameservers → Save
5. Wait up to an hour — Cloudflare emails you when it's active

**Simplest path (skip steps 2–4):** Buy the domain directly from [Cloudflare Registrar](https://cloudflare.com/products/registrar/) (at-cost, ~$8/yr for `.com`) — it's automatically managed by Cloudflare with no nameserver changes needed.

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

## 📄 License

MIT
