# Plexify

A tool to sync Spotify playlists to Plex music libraries.

## Features

- 🔍 Fetch songs from Spotify playlists
- 📋 Can take either a list of playlist IDs or find all public playlists by username
- 🎵 Extract track metadata (title, artist, album, duration, ISRC)
- 🎯 Match songs to Plex library using title/artist matching
- 📝 Create Plex playlists dynamically with matched songs
- 🧠 Retrieve the MusicBrainz id for missing songs to make it easier to find them
- 📝 Automatic release notes generation with PR information

## Prerequisites

- Spotify API credentials
- Plex Media Server with music library

## Quick Start

### Download Pre-built Binary

Download the latest release for your platform from [GitHub Releases](https://github.com/garry/plexify/releases):

```bash
# Linux (amd64)
wget https://github.com/garry/plexify/releases/latest/download/plexify-linux-amd64
chmod +x plexify-linux-amd64

# macOS (Intel)
wget https://github.com/garry/plexify/releases/latest/download/plexify-darwin-amd64
chmod +x plexify-darwin-amd64

# macOS (Apple Silicon)
wget https://github.com/garry/plexify/releases/latest/download/plexify-darwin-arm64
chmod +x plexify-darwin-arm64

# Windows
# Download plexify-windows-amd64.exe from the releases page
```

### Release Notes

Each release includes comprehensive release notes that automatically list:
- All merged pull requests since the last release
- PR details including title, author, and labels
- Download links for all supported platforms
- Installation and usage instructions

To preview release notes for the next version:
```bash
make release-notes-preview
```

To generate release notes for a specific version:
```bash
make release-notes VERSION=v1.0.0 PREVIOUS_TAG=v0.9.0
```

### 2. Spotify API Setup

1. Go to [Spotify Developer Dashboard](https://developer.spotify.com/dashboard)
2. Create a new application
3. Note your `Client ID` and `Client Secret`
4. Add `http://localhost:8080/callback` to your redirect URIs

### 3. Plex Setup

1. **Get your Plex Token**:
   - Go to [Plex Web](https://app.plex.tv/web/app)
   - Open Developer Tools (F12)
   - Go to Network tab
   - Look for requests to `plex.tv` and find the `X-Plex-Token` header
   - Or use the [Plex Token Finder](https://www.plexopedia.com/plex-media-server/general/plex-token/)
   - Or follow the instructions in [PLEX_AUTHENTICATION.md](PLEX_AUTHENTICATION.md)

2. **Find your Music Library Section ID**:
   - Go to your Plex server web interface
   - Navigate to your music library
   - The section ID is in the URL: `http://your-server:32400/web/index.html#!/media/plex/:/server/{server-id}/section/{section-id}/all`
   - Or check the Plex logs for section information

3. **Server ID (Optional - Auto-discovered)**:
   - The server ID can be automatically discovered from your Plex server
   - If auto-discovery fails, you can find it in the Plex Web UI URL or set it manually
   - Leave `PLEX_SERVER_ID` empty in your `.env` file to enable auto-discovery

### 4. Configuration

Copy the template environment file and fill in your values:

```bash
cp env.template .env
```

Edit `.env` with your configuration:

```env
# Spotify Configuration
SPOTIFY_CLIENT_ID=your_spotify_client_id_here
SPOTIFY_CLIENT_SECRET=your_spotify_client_secret_here
SPOTIFY_REDIRECT_URI=http://localhost:8080/callback

# Plex Configuration
PLEX_URL=http://your_plex_server:32400
PLEX_TOKEN=your_plex_token_here
PLEX_LIBRARY_SECTION_ID=your_music_library_section_id

# Playlist Configuration (Optional)
# Option 1: Set SPOTIFY_USERNAME to fetch all public playlists for a user
SPOTIFY_USERNAME=your_spotify_username_here

# Option 2: Comma-separated list of specific Spotify playlist IDs
SPOTIFY_PLAYLIST_ID=playlist_id_1,playlist_id_2,playlist_id_3


### 6. Configure Playlists (Optional)

You have two options for specifying which playlists to sync:

#### Option 1: Use Spotify Username (Recommended)
Set `SPOTIFY_USERNAME` in your `.env` file to fetch all public playlists for that user:
```env
SPOTIFY_USERNAME=your_spotify_username_here
```

#### Option 2: Use Specific Playlist IDs
1. Open Spotify and navigate to your playlist
2. Right-click and select "Share" → "Copy link to playlist"
3. The playlist ID is the string after the last `/` in the URL
   - Example: `https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M`
   - Playlist ID: `37i9dQZF1DXcBWIGoYBM5M`

### 6. Configure Multiple Playlists (Optional)

You can sync multiple Spotify playlists at once by providing a comma-separated list:

```env
# Single playlist
SPOTIFY_PLAYLIST_ID=37i9dQZF1DXcBWIGoYBM5M

# Multiple playlists
SPOTIFY_PLAYLIST_ID=37i9dQZF1DXcBWIGoYBM5M,37i9dQZF1DXcBWIGoYBM5N,37i9dQZF1DXcBWIGoYBM5O
```

Each Spotify playlist will be synced to a new Plex playlist with the exact same name as the original Spotify playlist.

## Usage

### Basic Usage

```bash
# Build and run with playlist ID(s) from .env
make run

# Or run directly
go run main.go

# Or run the built binary
./bin/plexify
```

### Command Line Options

```bash
# Use specific playlist ID(s) (overrides .env)
go run main.go -playlists 37i9dQZF1DXcBWIGoYBM5M

# Multiple playlists (comma-separated)
go run main.go -playlists 37i9dQZF1DXcBWIGoYBM5M,37i9dQZF1DXcBWIGoYBM5N,37i9dQZF1DXcBWIGoYBM5O

# Or with make
make run-playlist PLAYLIST_ID=37i9dQZF1DXcBWIGoYBM5M
```

### Example Output

```
Playlist: fresh fades 🔥
Description: fresh music, updated on Saturdays
Total tracks: 62
Owner: repeattofade

Songs in playlist (62 total):
================================================================================
  1. Bad Bunny - ALAMBRE PúA (ALAMBRE PúA)
  2. Mae Stephens - Tiny Voice (Tiny Voice)
  3. Georgia - Wanna Play (Wanna Play)
  ...
Successfully fetched 62 songs from Spotify playlist

================================================================================
MATCHING SONGS TO PLEX LIBRARY
================================================================================
Matching song 1/62: Bad Bunny - ALAMBRE PúA
Matching song 2/62: Mae Stephens - Tiny Voice
...

================================================================================
MATCHING RESULTS
================================================================================
  1. Bad Bunny - ALAMBRE PúA: ✅ ISRC match (Plex: Bad Bunny - ALAMBRE PúA)
  2. Mae Stephens - Tiny Voice: 🔍 Title/Artist match (Plex: Mae Stephens - Tiny Voice)
  3. Georgia - Wanna Play: ❌ No match
  ...

================================================================================
SUMMARY
================================================================================
Total songs: 62
ISRC matches: 45 (72.6%)
Title/Artist matches: 12 (19.4%)
No matches: 5 (8.1%)
Total matches: 57 (91.9%)

✅ Successfully created playlist: fresh fades 🔥 (ID: 12345)
✅ Successfully added 57 tracks to playlist: fresh fades 🔥

================================================================================
MISSING TRACKS SUMMARY
================================================================================
Tracks not found in Plex library (5 total):
--------------------------------------------------------------------------------
  1. Diplo - Get It Right
     Track ID: 4Qv9uaS4tPFlmG7Iac9uQJ
     ISRC: (not available)

  2. Some Artist - Some Song
     Track ID: 7x8dJ7q9K2L3M4N5O6P7Q8
     ISRC: USRC12345678
     MusicBrainz ID: 12345678-1234-1234-1234-123456789012 (https://musicbrainz.org/recording/12345678-1234-1234-1234-123456789012)

  3. Another Artist - Another Song
     Track ID: 1A2B3C4D5E6F7G8H9I0J1K2L
     ISRC: (not available)
     MusicBrainz ID: (not found)

**Note:** The Missing Tracks Summary shows tracks that couldn't be found in your Plex library, including their Spotify Track ID, ISRC (International Standard Recording Code), and MusicBrainz ID when available. MusicBrainz IDs include direct links to the MusicBrainz website for easy access to additional track information. This information can be helpful for manually adding these tracks to your Plex library or for troubleshooting matching issues. MusicBrainz IDs are automatically looked up using the ISRC when available, or by searching for the artist and title combination.

## MusicBrainz Integration

Plexify includes integration with the MusicBrainz database to provide additional track identification information. When tracks are not found in your Plex library, Plexify will automatically:

1. **Search by ISRC** (if available): Uses the International Standard Recording Code to find the exact track in MusicBrainz
2. **Search by Artist/Title** (fallback): If ISRC is not available or not found, searches by artist and title combination

The MusicBrainz ID can be used to:
- Look up detailed track information on the MusicBrainz website
- Find alternative versions or releases of the same track
- Get additional metadata like recording dates, genres, and more
- Use with other music services that support MusicBrainz IDs

**API Rate Limiting:** MusicBrainz has rate limiting in place. Plexify respects these limits and will make requests at a reasonable pace to avoid being blocked.

## Configuration

Plexify supports multiple ways to configure settings, with the following hierarchy (highest priority first):

1. **CLI Flags** - Override everything else
2. **Environment Variables** - Loaded from `.env` file or system environment
3. **System Environment Variables** - Fallback if `.env` file doesn't exist

### CLI Flags

All environment variables can be set via CLI flags using the same names:

```bash
# Spotify configuration
./plexify -SPOTIFY_CLIENT_ID=your_client_id -SPOTIFY_CLIENT_SECRET=your_secret
./plexify -SPOTIFY_USERNAME=your_username
./plexify -SPOTIFY_PLAYLIST_ID=37i9dQZF1DXcBWIGoYBM5M,37i9dQZF1DXcBWIGoYBM5N

# Plex configuration
./plexify -PLEX_URL=http://your-server:32400 -PLEX_TOKEN=your_token
./plexify -PLEX_LIBRARY_SECTION_ID=6
./plexify -PLEX_SERVER_ID=your_server_id

# Legacy flags (still supported)
./plexify -username your_username -playlists playlist_id1,playlist_id2
```

### Environment Variables

Create a `.env` file in the same directory as the binary:

```env
# Spotify API credentials
SPOTIFY_CLIENT_ID=your_client_id
SPOTIFY_CLIENT_SECRET=your_client_secret
SPOTIFY_REDIRECT_URI=http://localhost:8080/callback
SPOTIFY_USERNAME=your_spotify_username
SPOTIFY_PLAYLIST_ID=37i9dQZF1DXcBWIGoYBM5M,37i9dQZF1DXcBWIGoYBM5N

# Plex Configuration
PLEX_URL=http://your_plex_server:32400
PLEX_TOKEN=your_plex_token_here
PLEX_LIBRARY_SECTION_ID=your_music_library_section_id
PLEX_SERVER_ID=your_server_id
```

### Configuration Priority Example

If you have:
- `SPOTIFY_USERNAME=user1` in your `.env` file
- `SPOTIFY_USERNAME=user2` as a system environment variable
- `./plexify -SPOTIFY_USERNAME=user3`

The final value will be `user3` (CLI flag takes precedence).

## Development

### Building for Release

To build for all platforms:

```bash
make build-release
```

This creates binaries for:
- Linux (amd64, arm64)
- macOS (amd64, arm64) 
- Windows (amd64, arm64)

### Running Tests

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage

# Run tests with verbose output
make test-verbose
```

### Code Quality

```bash
# Format code
make format

# Vet code
make vet

# Run all checks (format, vet, test)
make check
```

### Release Process

This repository uses automatic releases when pull requests are merged with specific labels:
- `patch` - Bug fixes (increments patch version)
- `minor` - New features (increments minor version)
- `major` - Breaking changes (increments major version)

For detailed information about the release process, see [RELEASE.md](RELEASE.md).

## Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `SPOTIFY_CLIENT_ID` | Spotify API Client ID | Yes |
| `SPOTIFY_CLIENT_SECRET` | Spotify API Client Secret | Yes |
| `SPOTIFY_PLAYLIST_ID` | Comma-separated list of Spotify playlist IDs | Yes |
| `SPOTIFY_REDIRECT_URI` | Spotify OAuth redirect URI | No (default: http://localhost:8080/callback) |
| `PLEX_URL` | Plex Media Server URL | Yes |
| `PLEX_TOKEN` | Plex authentication token | Yes |
| `PLEX_LIBRARY_SECTION_ID` | Plex music library section ID | Yes |
| `PLEX_SERVER_ID` | Plex server ID | No (auto-discovered from server if not set) |
| `LOG_LEVEL` | Logging level (debug, info, warn, error) | No (default: info) |

## Matching Functions

Plexify uses a sophisticated multi-step matching system to find songs from Spotify in your Plex library. The matching happens in a specific order, with each step trying different strategies to find the best match.

### Matching Order and Rules

#### 1. **Exact Title/Artist Match** (First Priority)
**When it applies:** All songs (highest priority for reliability)
**What it does:** Searches using the exact title and artist from Spotify without any modifications
**Rules:**
- Tries combined search: `"Song Title Artist Name"`
- Falls back to title-only search: `"Song Title"`
- Falls back to artist-only search: `"Artist Name"`
- Returns immediately if exact match is found (most reliable)

#### 2. **Single Quote Handling** (Second Priority)
**When it applies:** Songs with apostrophes or single quotes in the title or artist name
**What it does:** Tries different variations of the quote characters to handle encoding differences
**Rules:**
- Original title with quotes: `"Don't Stop Believin'"`
- Remove all quotes: `"Dont Stop Believin"`
- Replace with backtick: `"Don`t Stop Believin``"`
- Replace with prime symbol: `"Don′t Stop Believin′"`
- Replace with different quote: `"Don't Stop Believin'"`
- Expand contractions: `"Do not Stop Believin"`, `"Do not Stop Believin is"`

#### 3. **Bracket Removal** (Third Priority)
**When it applies:** Songs with text in parentheses, square brackets, or curly brackets
**What it does:** Removes bracketed content and searches again
**Rules:**
- `"Song Title (feat. Artist)"` → `"Song Title"`
- `"Song Title [Remix]"` → `"Song Title"`
- `"Song Title {Live}"` → `"Song Title"`
- Cleans up extra spaces after removal

#### 4. **Featuring Removal** (Fourth Priority)
**When it applies:** Songs with featuring/feat/ft text in the title
**What it does:** Removes featuring information and searches again
**Rules:**
- `"Song Title featuring Artist"` → `"Song Title"`
- `"Song Title feat. Artist"` → `"Song Title"`
- `"Song Title ft. Artist"` → `"Song Title"`