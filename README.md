# InfoKeep

InfoKeep is a personal, self-hosted bookmarking and knowledge base application. It allows you to save bookmarks, write notes, store recipes, create check-lists, and draw diagrams—all within a single, fast interface.

## Features

- **Bookmarks**: Save links with automatic fetching of titles, thumbnails, and favicons.
- **Notes**: Write rich text notes.
- **Recipes**: Import recipes directly from cooking URLs.
- **Lists and Check-lists**: Maintain rated lists and check-lists with ease.
- **Drawings**: Create sketches or mind maps.
- **Media**: Upload images and videos.
- **Reminders**: Set one-time or recurring reminders with Web Push notifications.
- **Global Search & Tags**: Quickly organize and find anything globally.
- **Dashboard Pinning**: Keep track of your most important items instantly on your dashboard.
- **Sharing**: Share live links of any of your items publicly. 
- **PWA Ready**: Installable on Android and Desktop, featuring Android Share Sheet integration to quickly save links from other apps.
- **Cloud Backup**: Automated integrations with pCloud and Google Drive.

## Quick Start (Docker)

InfoKeep is designed to be easily self-hosted using Docker. The application is entirely contained within a lightweight container, and uses an embedded SQLite database, meaning no separate database container is required!

### 1. docker-compose.yml

Create a `docker-compose.yml` file on your server with the following contents:

```yaml
services:
  infokeep:
    # Use the pre-built image, or build from source using: build: .
    image: bloubanibou/infokeep:latest # NOTE: Change this to the actual image name once published
    container_name: infokeep
    restart: unless-stopped
    ports:
      # Map the host port (e.g., 8989) to the container port (8080)
      - "8989:8080"
    volumes:
      # Persist the SQLite database (Create a blank infokeep.db file first!)
      - ./infokeep.db:/app/infokeep.db
      # Persist uploaded media
      - ./uploads:/app/web/static/uploads
    environment:
      # Set the internal port the Go server runs on
      - PORT=8080
      # Inform InfoKeep where the database is located within the container
      - DB_PATH=/app/infokeep.db
      # Timezone Syncing for Reminders (Optional)
      - TZ=America/New_York
```

### 2. Create Required Files & Folders

Before starting the container, ensure the files and directories for the volumes exist to prevent Docker from creating them as root directories incorrectly:

```bash
# Create an empty database file
touch infokeep.db

# Create the uploads directory
mkdir uploads
```

### 3. Start the Application

Run the following command to download and start InfoKeep in the background:

```bash
docker compose up -d
```

InfoKeep will now be accessible at `http://localhost:8989` (or your server's IP address).

## Environment Variables

| Variable | Default Value | Description |
| :--- | :--- | :--- |
| `PORT` | `8080` | The internal port the web server listens on. |
| `DB_PATH`| `infokeep.db` | The path to the SQLite database file. |
| `TZ` | `UTC` | The timezone for the container, necessary for accurate Reminders. |
| `VAPID_PUBLIC_KEY` | (Auto-generated) | Optional. Can be injected via environment instead of database generation. |
| `VAPID_PRIVATE_KEY`| (Auto-generated) | Optional. Can be injected via environment instead of database generation. |

## Building from Source

If you wish to build the Docker image yourself instead of pulling a pre-built image:

1. Clone this repository.
2. In your `docker-compose.yml`, replace `image: ...` with `build: .`
3. Run `docker compose up -d --build`
