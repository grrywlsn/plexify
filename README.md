# Plexify

A tool to sync Spotify playlists to Plex music libraries.

## Korean Track Matching Bug Investigation

### Problem Description
A bug was reported where a Korean track "Like this" by "박혜진 Park Hye Jin" was being incorrectly matched with high confidence to multiple unrelated songs, particularly Taylor Swift's "the lakes - bonus track".

### Investigation Results
Comprehensive tests were written to investigate this bug. The findings show that:

1. **Current code works correctly** - The confidence calculation between "the lakes - bonus track" by "Taylor Swift" and "Like this" by "박혜진 Park Hye Jin" is 0.259, which is below the 0.6 threshold, so no match is returned.

2. **Most likely cause** - The Korean track in the actual Plex library likely has different metadata than "Like this". Specifically, if the track has a title similar to "the lakes", such as:
   - "The Lake" (confidence: 0.671 - would match!)
   - "The Lakes" (confidence: 0.749 - would match but rejected due to artist similarity check)
   - "THE LAKES" (confidence: 0.749 - would match but rejected due to artist similarity check)

3. **The bug occurs when** - The Korean track has a title that's very similar to "the lakes" and the confidence score exceeds the 0.6 threshold.

### Test Files Created
The following test files were created to investigate this bug:

- `TestKoreanTrackConfidenceCalculationBug` - Tests confidence calculation
- `TestKoreanTrackFullSearchFlowBug` - Tests the full search flow
- `TestKoreanTrackSearchMethodsBug` - Tests individual search methods
- `TestKoreanTrackBugInvestigation` - Comprehensive investigation
- `TestKoreanTrackBugSummary` - Summary of findings
- `TestKoreanTrackMostLikelyBugCause` - Tests different metadata variations

### Recommendations
1. Check the actual metadata of the Korean track in the Plex library
2. If it has a similar title to "the lakes", consider adding additional checks
3. Consider adding artist similarity requirements for high-confidence matches
4. Consider adding a minimum artist similarity threshold

### Running the Tests
```bash
# Run all Korean track bug tests
go test -v ./plex -run TestKoreanTrack

# Run specific test
go test -v ./plex -run TestKoreanTrackMostLikelyBugCause
```

## Features

- 🔍 Fetch songs from Spotify playlists
- 🎵 Extract track metadata (title, artist, album, duration, ISRC)
- 🎯 Match songs to Plex library using ISRC and title/artist matching
- 📝 Create Plex playlists dynamically with matched songs
- 🔧 Environment-based configuration
- 🚀 CLI-friendly for cronjob automation
- 📦 Well-structured Go code following best practices

## Prerequisites

- Go 1.21 or higher
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

### Build from Source

If you prefer to build from source:

## Setup

### 1. Clone and Setup

```bash
git clone <your-repo-url>
cd plexify
make setup
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

# Plex Configuration (for future use)
PLEX_URL=http://your_plex_server:32400
PLEX_TOKEN=your_plex_token_here
PLEX_LIBRARY_SECTION_ID=your_music_library_section_id

# Playlist Configuration (Optional)
# Option 1: Set SPOTIFY_USERNAME to fetch all public playlists for a user
SPOTIFY_USERNAME=your_spotify_username_here

# Option 2: Comma-separated list of specific Spotify playlist IDs
SPOTIFY_PLAYLIST_ID=playlist_id_1,playlist_id_2,playlist_id_3

# Application Configuration
LOG_LEVEL=info
```

### 5. Build and Test

```bash
# Build for current platform
make build

# Run tests
make test

# Check version
./bin/plexify -version
```

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
```

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
- Uses the last occurrence of featuring text (most common pattern)

#### 5. **Title Normalization** (Fifth Priority)
**When it applies:** Songs with dashes in the title
**What it does:** Converts dashes to parentheses format
**Rules:**
- `"Song Title - Remix"` → `"Song Title (Remix)"`
- `"Song Title - Version - Edit"` → `"Song Title (Version) (Edit)"`
- Handles multiple dashes by wrapping each part in parentheses

#### 6. **With Removal** (Sixth Priority)
**When it applies:** Songs with "with" in the title
**What it does:** Removes "with" and following text, also cleans up trailing dashes
**Rules:**
- `"Song Title with Artist"` → `"Song Title"`
- `"Song Title - with Artist"` → `"Song Title"`
- `"with Artist - Song Title"` → `"Song Title"`
- Only removes "with" as a whole word (not "without", "within", etc.)
- Automatically removes trailing dashes and spaces after "with" removal

#### 7. **Common Suffix Removal** (Seventh Priority)
**When it applies:** Songs with common suffixes like "bonus track", "remix", "extended", etc.
**What it does:** Removes common suffixes to match the base song title
**Rules:**
- `"Song Title - bonus track"` → `"Song Title"`
- `"Song Title - Remix"` → `"Song Title"`
- `"Song Title - Extended"` → `"Song Title"`
- `"Song Title - Radio Edit"` → `"Song Title"`
- `"Song Title - Live"` → `"Song Title"`
- `"Song Title - Acoustic"` → `"Song Title"`
- `"Song Title (Bonus Track)"` → `"Song Title"`
- `"Song Title (Remix)"` → `"Song Title"`
- Handles both dash and parentheses formats
- Automatically removes trailing dashes and spaces after suffix removal

#### 8. **Full Library Search** (Last Resort)
**When it applies:** When all other methods fail
**What it does:** Searches through every track in your Plex library
**Rules:**
- Most expensive operation (slowest)
- Used only when other methods can't find tracks that should exist
- Still applies the same matching logic to find the best match

### How Matches Are Scored

For each search result, Plexify calculates a confidence score using multiple similarity checks:

#### Title Similarity (70% weight)
The system tries **six different title variations** and uses the best score:
1. **Original title** comparison
2. **Bracket-removed** title comparison  
3. **Featuring-removed** title comparison
4. **Normalized** title comparison (dashes → parentheses)
5. **With-removed** title comparison
6. **Suffix-removed** title comparison (bonus track, remix, etc.)

#### Artist Similarity (30% weight)
Direct comparison of artist names

#### Similarity Calculation
- **Exact match:** 100% confidence
- **Substring match:** Percentage of the longer string covered
- **Word-level match:** Percentage of matching words
- **Length similarity:** How close the string lengths are

#### Confidence Threshold
- **Minimum score:** 60% confidence required for a match
- **Perfect match:** Returns immediately (100% confidence)
- **Artist similarity check:** When title similarity is very high (>90%), artist similarity must also be reasonable (≥30%) to prevent incorrect matches

### Example Matching Process

For a Spotify song: `"Don't Stop Believin' (feat. Journey)"`

1. **Exact Match:** Searches for `"Don't Stop Believin' (feat. Journey)"` with exact title/artist
2. **Single Quote Handling:** Tries `"Dont Stop Believin"`, `"Don`t Stop Believin``"`, etc.
3. **Bracket Removal:** Searches for `"Don't Stop Believin'"`
4. **Featuring Removal:** Searches for `"Don't Stop Believin'"`
5. **Title Normalization:** No dashes to normalize
6. **With Removal:** No "with" to remove
7. **Suffix Removal:** No common suffixes to remove
8. **Full Library Search:** Searches entire library if needed

For a Spotify song: `"the lakes - bonus track"`

1. **Exact Match:** Searches for `"the lakes - bonus track"`
2. **Single Quote Handling:** No quotes to handle
3. **Bracket Removal:** No brackets to remove
4. **Featuring Removal:** No featuring to remove
5. **Title Normalization:** Converts to `"the lakes (bonus track)"`
6. **With Removal:** No "with" to remove
7. **Suffix Removal:** Removes "bonus track" → searches for `"the lakes"`
8. **Full Library Search:** Searches entire library if needed

### Test Cases

The matching logic includes comprehensive test cases to ensure reliability. For example, the system correctly handles:

- **"Neon Moon - with Kacey Musgraves" by Brooks & Dunn** → matches **"Neon Moon" by Brooks & Dunn**
- **"the lakes - bonus track" by Taylor Swift** → matches **"the lakes" by Taylor Swift**
- **"Don't Stop Believin'" by Journey** → handles apostrophes and contractions
- **"Song Title (feat. Artist)"** → matches **"Song Title"** after bracket removal
- **"Song Title - Remix"** → matches **"Song Title (Remix)"** after normalization
- **"Song Title - Extended"** → matches **"Song Title"** after suffix removal

**Important:** The system includes safeguards to prevent incorrect matches. For example:
- **"The Lakes" by "Some Other Artist"** → **NO match** (correctly rejected due to artist mismatch)
- **"The Lakes" by "Taylor Swift"** → **matches** (correctly accepted due to both title and artist similarity)

These test cases help ensure the matching logic works correctly for real-world scenarios and can catch regressions when making improvements.

### Improving Match Success

To improve matching success rates:

1. **Ensure consistent naming** in your Plex library
2. **Use standard artist names** (avoid "feat.", "ft.", etc. in artist fields)
3. **Clean up track titles** by removing unnecessary brackets or featuring text
4. **Check for encoding issues** with special characters
5. **Verify your music library** has the songs you're trying to match

The matching system is designed to be flexible and handle common variations, but the quality of your Plex library metadata directly affects match success rates.
