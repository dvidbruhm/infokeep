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
| ☁️ **Cloud Backup** | Automatic scheduled backups of your database to pCloud |

---

## 🚀 Getting Started

InfoKeep comes with helpful automation scripts for easy building and running. You can use standard `make` (Linux/Mac/WSL) or the native `.\manage.ps1` script (Windows).

### Built-in Run Commands
Run `make help` or `.\manage.ps1 help` to see all commands:
- `build` / `run` / `clean` - Local Go commands
- `docker-build` / `docker-up` / `docker-down` - Standard Docker execution
- `docker-rebuild` - Fully rebuilds docker containers and restarts them without cache
- `docker-logs` - Tails the Docker container logs

### Option 1 — Run locally (development)

**Requirements:** Go 1.24+, GCC (for CGO / SQLite)

```bash
git clone https://github.com/dvidbruhm/infokeep.git
cd infokeep

# Linux/Mac
make run 

# Windows
.\manage.ps1 run
```

The app starts on **http://localhost:8080**.

### Option 2 — Docker Compose

```bash
git clone https://github.com/dvidbruhm/infokeep.git
cd infokeep

# Linux/Mac
make docker-up

# Windows
.\manage.ps1 docker-up
```

The app will be available at **http://localhost:8989**.

> Data is persisted via Docker volumes — your database and uploads survive container restarts. To rebuild everything from scratch, use `make docker-rebuild`.

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
│       ├── pcloud.go           # pCloud OAuth2 + backup logic
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

All configuration is done via environment variables. Create a `.env` file in the project root (it's already gitignored):

```bash
# .env
PCLOUD_CLIENT_ID=your_client_id
PCLOUD_CLIENT_SECRET=your_client_secret
GDRIVE_CLIENT_ID=your_client_id
GDRIVE_CLIENT_SECRET=your_client_secret
```

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | Port the server listens on |
| `PCLOUD_CLIENT_ID` | *(empty)* | pCloud OAuth2 app client ID |
| `PCLOUD_CLIENT_SECRET` | *(empty)* | pCloud OAuth2 app client secret |
| `GDRIVE_CLIENT_ID` | *(empty)* | Google Drive OAuth2 client ID |
| `GDRIVE_CLIENT_SECRET` | *(empty)* | Google Drive OAuth2 client secret |

The database file (`infokeep.db`) is created automatically in the working directory on first run.

### Using the `.env` file

**With Docker Compose** — it's loaded automatically (configured in `docker-compose.yml`):

```bash
docker compose up -d
```

**With `go run` (PowerShell)** — load it manually before running:

```powershell
Get-Content .env | ForEach-Object { if ($_ -match '^([^#].+?)=(.*)$') { [System.Environment]::SetEnvironmentVariable($matches[1], $matches[2]) } }
go run .
```

---

## ☁️ Cloud Backup (pCloud)

InfoKeep can automatically back up your SQLite database to [pCloud](https://www.pcloud.com/) on a configurable schedule.

### Step 1 — Register a pCloud app

1. Go to [my.pcloud.com/oauth2/register](https://my.pcloud.com/oauth2/register)
2. Fill in the app details:
   - **App Name**: `InfoKeep Backup` (or anything you like)
   - **Redirect URI**: `https://yourdomain.com/settings/pcloud/callback`
     - For local dev: `http://localhost:8080/settings/pcloud/callback`
3. After creating the app, copy the **Client ID** and **Client Secret**

### Step 2 — Add credentials to `.env`

Open the `.env` file in the project root and fill in:

```bash
PCLOUD_CLIENT_ID=your_client_id_here
PCLOUD_CLIENT_SECRET=your_client_secret_here
```

### Step 3 — Restart the server

```bash
# Docker
docker compose down && docker compose up -d

# Or local
# (re-run the PowerShell env loading + go run)
```

### Step 4 — Link your account

1. Go to **Settings** in InfoKeep
2. Find the **Cloud Backup (pCloud)** section
3. Click **Link pCloud Account**
4. Log in to pCloud and approve the access
5. You'll be redirected back to Settings with a "Connected" status

### Step 5 — Configure backup interval

- Set the number of days between automatic backups (default: **7 days**)
- Click **Save**
- Use **Backup Now** to trigger an immediate backup

Backups are saved to a `/InfoKeep Backups/` folder on your pCloud with the filename `infokeep_backup_YYYY-MM-DD.db`.

---

## ☁️ Cloud Backup (Google Drive)

InfoKeep can also back up your database to [Google Drive](https://drive.google.com/).

### Step 1 — Create Google OAuth2 credentials

1. Go to [Google Cloud Console](https://console.cloud.google.com/apis/credentials)
2. Create a new project (or use an existing one)
3. Enable the **Google Drive API** under APIs & Services → Library
4. Go to **Credentials** → **Create Credentials** → **OAuth client ID**
   - Application type: **Web application**
   - Authorized redirect URI: `https://yourdomain.com/settings/gdrive/callback`
     - For local dev: `http://localhost:8080/settings/gdrive/callback`
5. Copy the **Client ID** and **Client Secret**

### Step 2 — Add credentials to `.env`

```bash
GDRIVE_CLIENT_ID=your_client_id_here
GDRIVE_CLIENT_SECRET=your_client_secret_here
```

### Step 3 — Link your account

1. Restart the server
2. Go to **Settings** → **Cloud Backup (Google Drive)**
3. Click **Link Google Drive**
4. Sign in with Google and grant access
5. You'll be redirected back with a "Connected" status

Backups are saved to an `InfoKeep Backups` folder on your Google Drive. The backup interval is shared with pCloud (configured in the pCloud section).

---

## 🎨 Adding a New Theme

To add a new theme, update **6 places** across 2 files. Use an existing theme (e.g. `dracula`) as a reference in each section.

### 1. CSS Variables — `web/templates/layout.html`

Add a `[data-theme="your-theme"]` block defining all 10 core CSS variables:

```css
[data-theme="your-theme"] {
    --app-bg: #...;          /* Page background */
    --sidebar-bg: #...;      /* Sidebar background */
    --card-bg: #...;         /* Card / box background */
    --text-main: #...;       /* Body text */
    --text-strong: #...;     /* Headings / strong text */
    --text-muted: #...;      /* Secondary / muted text */
    --border-color: #...;    /* Borders and dividers */
    --card-shadow: ...;      /* Box shadow for cards */
    --card-hover-shadow: ...; /* Box shadow on hover */
    --input-bg: #...;        /* Input / select background */
}
```

### 2. Per-Theme Overrides — `web/templates/layout.html`

Add a block for menu active state, link colors, `button.is-link`, and semantic tag colors:

```css
/* Your Theme Overrides */
[data-theme="your-theme"] .menu-list a.is-active {
    background-color: #...; color: #... !important;
}
[data-theme="your-theme"] .is-link,
[data-theme="your-theme"] .has-text-link { color: #... !important; }
[data-theme="your-theme"] .button.is-link {
    background-color: #... !important; color: #... !important;
}
[data-theme="your-theme"] .tag.is-info, ...   { color: #... !important; }
[data-theme="your-theme"] .tag.is-success, ... { color: #... !important; }
[data-theme="your-theme"] .tag.is-warning, ... { color: #... !important; }
[data-theme="your-theme"] .tag.is-danger, ...  { color: #... !important; }
[data-theme="your-theme"] .menu-list a:hover {
    background-color: rgba(255,255,255,0.06) !important;
}
```

### 3. Common Utility Selectors — `web/templates/layout.html`

Add your theme to the existing multi-selector blocks (for dark themes only, skip for light themes):

- **`.button.is-white`, `.button.is-light`, `.tag.is-light`** — search for `Common Utility Overrides`
- **`.has-background-light`, `.has-background-white`** — same section, just below

### 4. Tag Variables — `web/templates/layout.html`

Add a `[data-theme="your-theme"]` block in the `Tag Standardization` section:

```css
[data-theme="your-theme"] {
    --tag-bg: #...;  /* use border-color or similar */
    --tag-fg: #...;  /* use text-main */
}
```

### 5. Theme Picker Card — `web/templates/settings.html`

Add a card in the theme grid (use the `--app-bg` color for the preview circle):

```html
<div class="column is-4 has-text-centered">
    <div class="card p-4 is-clickable" onclick="applyTheme('your-theme')"
        style="cursor: pointer; border: 2px solid transparent;" id="theme-your-theme">
        <div class="theme-preview" style="background: #APP_BG; border-color: #BORDER;"></div>
        <p class="has-text-weight-bold">Your Theme</p>
    </div>
</div>
```

### 6. Theme Editor Defaults — `web/templates/settings.html`

Add an entry to the `themeDefaults` JavaScript object (inside the `<script>` tag):

```javascript
'your-theme': { '--app-bg': '#...', '--sidebar-bg': '#...', '--card-bg': '#...', ... }
```

> **Tip:** After adding a theme, run `go build ./...` to verify the templates parse correctly.

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
