# Plexify

A tool to sync Spotify playlists to playlists on your Plex server.

Can be run manually as a CLI, or scheduled as a cron task.

## Features

- 🔍 Fetch songs from Spotify playlists
- 📋 Can take either a [list of Spotify playlists, or find all public playlists by username](#5-finding-playlists)
- 🎵 Extract track metadata (title, artist, album, duration, ISRC)
- 🎯 Match Spotify songs to Plex library using title/artist [using pre-defined rules](#matching-rules)
- 📝 Create Plex playlists dynamically with matched songs, or update existing playlists
- 🧠 [Retrieve the MusicBrainz id](#musicbrainz-integration) for missing songs to make it easier to find them

## Prerequisites

- Spotify API credentials (**not** your username and password)
- Plex Media Server with music library, and the `X-Plex-Token` token

## Quick Start

### 1. Download Pre-built Binary

Download the latest release for your platform from [GitHub Releases](https://github.com/grrywlsn/plexify/releases):

**Linux (amd64):**

```bash
wget https://github.com/grrywlsn/plexify/releases/latest/download/plexify-linux-amd64 -O plexify && chmod +x plexify
```

**macOS (Intel):**

```bash
wget https://github.com/grrywlsn/plexify/releases/latest/download/plexify-darwin-amd64 -O plexify && chmod +x plexify
```

**macOS (Apple Silicon):**

```bash
wget https://github.com/grrywlsn/plexify/releases/latest/download/plexify-darwin-arm64 -O plexify && chmod +x plexify
```

**Windows:**
Download `plexify-windows-amd64.exe` from the [releases page](https://github.com/grrywlsn/plexify/releases) and rename to `plexify.exe`

### 2. Spotify API Setup

1. Go to [Spotify Developer Dashboard](https://developer.spotify.com/dashboard)
2. Create a new application
3. Note your `Client ID` and `Client Secret`
4. Add `http://localhost:8080/callback` to your redirect URIs

### 3. Plex Setup

1. **Get your Plex Token**:

   - Go to [Plex Web](https://app.plex.tv/web/app)
   - Open Developer Tools
   - Go to Network tab
   - Look for requests to `plex.tv` and find the `X-Plex-Token` header
   - Or use the [Plex Token Finder](https://www.plexopedia.com/plex-media-server/general/plex-token/)
2. **Find your Music Library Section ID**:

   - Go to your Plex server web interface
   - Navigate to your music library
   - The section ID is in the URL as the `source`: `https://app.plex.tv/desktop/#!/media/abcdefg12345678/com.plexapp.plugins.library?source=6`
   - Or check the Plex logs for section information
3. **Server ID (Optional - Auto-discovered)**:

   - The server ID can be automatically discovered from your Plex server
   - If `PLEX_SERVER_ID` is unset, it will attempt auto-discovery
   - If auto-discovery fails, you can find it in the Plex Web UI URL and set it manually

### 4. Configuration

The variables for configuration are:

```env
# Spotify Configuration
SPOTIFY_CLIENT_ID=your_spotify_client_id_here
SPOTIFY_CLIENT_SECRET=your_spotify_client_secret_here
SPOTIFY_REDIRECT_URI=http://localhost:8080/callback

# Plex Configuration
PLEX_URL=http://your_plex_server:32400
PLEX_TOKEN=your_plex_token_here
PLEX_LIBRARY_SECTION_ID=your_music_library_section_id

# Playlist Configuration
# Option 1: Set SPOTIFY_USERNAME to fetch all public playlists for a user
SPOTIFY_USERNAME=your_spotify_username_here

# Option 2: Comma-separated list of specific Spotify playlist IDs
SPOTIFY_PLAYLIST_ID=playlist_id_1,playlist_id_2,playlist_id_3
```

These can be set either as environment variables, loaded from a `.env` file, or passed into the binary as flags like `plexify --SPOTIFY_USERNAME=myname`

### 5. Finding playlists

You can sync multiple Spotify playlists at once by providing a comma-separated list:

```env
# Single playlist
SPOTIFY_PLAYLIST_ID=37i9dQZF1DXcBWIGoYBM5M

# Multiple playlists
SPOTIFY_PLAYLIST_ID=37i9dQZF1DXcBWIGoYBM5M,37i9dQZF1DXcBWIGoYBM5N,37i9dQZF1DXcBWIGoYBM5O
```

* Open Spotify and navigate to your playlist
* Right-click and select "Share" → "Copy link to playlist"
* The playlist ID is the string after the last `/` in the URL
  - Example: `https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M`
  - Playlist ID: `37i9dQZF1DXcBWIGoYBM5M`

Or you can provide a Spotify username and all public playlists will be found and synced:

```env
SPOTIFY_USERNAME=your_spotify_username_here
```

Each Spotify playlist will be synced to a new Plex playlist with the exact same name as the original. If a playlist of the same name exists in Plex, it will update it to match the matched Spotify tracks - this **will remove other songs if they exist**.

## Results

### Example Output

```
Playlist: my favourite songs
Description: what I'm listening to right now
Total tracks: 62
Owner: myspotifyname

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

✅ Successfully created playlist: my favourite songs (ID: 12345)
✅ Successfully added 57 tracks to playlist: my favourite songs

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
```

**Note:** The Missing Tracks Summary shows tracks that couldn't be found in your Plex library, including their Spotify Track ID, ISRC (International Standard Recording Code), and MusicBrainz ID when available.

## MusicBrainz Integration

Plexify includes integration with the MusicBrainz database to provide additional track identification information. When tracks are not found in your Plex library, plexify will automatically:

1. **Search by ISRC** (if available): Uses the International Standard Recording Code to find the exact track in MusicBrainz
2. **Search by Artist/Title** (fallback): If ISRC is not available or not found, searches by artist and title combination

The MusicBrainz ID can be used to:

- Look up detailed track information on the MusicBrainz website
- Find alternative versions or releases of the same track
- Get additional metadata like recording dates, genres, and more
- Use with other music services that support MusicBrainz IDs
- Add to Lidarr to search for an exact match or fix the metadata assigned to your files

**API Rate Limiting:** MusicBrainz has rate limiting in place. Plexify respects these limits and will make requests at a reasonable pace to avoid being blocked.

## Configuration

Plexify supports multiple ways to configure settings, with the following hierarchy (highest priority first):

1. **CLI Flags** - Override everything else
2. **Environment Variables** - Loaded from `.env` file
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

## Matching Rules

The matching rules used by plexify will evolve and improve as it is used on more playlists and libraries.

The matching happens in a specific order, with each step trying different strategies to find the best match.

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

#### 5. **Title Normalization** (Fifth Priority)

**When it applies:** Songs with dashes in the title that might be formatted differently in Plex
**What it does:** Converts dash-separated titles to parentheses format for better matching
**Rules:**

- `"Mood Ring - Pride Remix"` → `"Mood Ring (Pride Remix)"`
- `"Song Title - Extended - Live"` → `"Song Title (Extended) (Live)"`
- Handles multiple dashes by converting each to separate parentheses

#### 6. **"With" Removal** (Sixth Priority)

**When it applies:** Songs with "with" in the title that might be formatted differently
**What it does:** Removes "with" and any text after it from the title
**Rules:**

- `"Song Title with Artist"` → `"Song Title"`
- `"With Artist - Song Title"` → `"Song Title"`
- Cleans up trailing dashes and spaces after removal

#### 7. **Common Suffixes Removal** (Seventh Priority)

**When it applies:** Songs with common suffixes that might not be present in Plex
**What it does:** Removes common track suffixes and variations
**Rules:**

- `"Song Title - Bonus Track"` → `"Song Title"`
- `"Song Title - Remix"` → `"Song Title"`
- `"Song Title - Extended"` → `"Song Title"`
- `"Song Title - Radio Edit"` → `"Song Title"`
- `"Song Title - Live"` → `"Song Title"`
- `"Song Title - Acoustic"` → `"Song Title"`
- `"Song Title (Bonus Track)"` → `"Song Title"`
- `"Song Title (Remix)"` → `"Song Title"`
- And many more variations (clean, explicit, demo, instrumental, etc.)

#### 8. **Full Library Search** (Eighth Priority - Fallback)

**When it applies:** When all other matching strategies fail
**What it does:** Searches through the entire music library to find potential matches
**Rules:**

- Uses the most comprehensive search method
- Applies similarity scoring to find the best match
- Used as a last resort when other methods don't find matches

## Matching issues

If you run into issues where plexify will not match a song that you know is in your Plex library, [please raise an issue in this repo](https://github.com/grrywlsn/plexify/issues), and include:

- the artist name and track name from Spotify
- the artist name and track name from your Plex

Please copy/paste them **exactly** as they appear in each source, so that the matching can be tested.

