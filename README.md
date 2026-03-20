# Plexify

A tool to sync playlists from a **[music-social](https://github.com/mastodon-site/musicsocial)** instance into Plex.

Run it as a CLI or on a schedule (e.g. cron). The catalog is read from your music-social instance over HTTPS; you only need Plex credentials plus `MUSIC_SOCIAL_URL`.

**Stateless:** Each run is independent. Plexify does not write a local database, cache file, or sync manifest. It reads the current music-social playlists, matches against Plex, and updates Plex playlists over the API. The only durable “state” is whatever Plex already stores for those playlists. Optional `.env` in the working directory is just configuration input (same as environment variables), not application state carried between runs.

> [!IMPORTANT]
> **Public vs unlisted:** `MUSIC_SOCIAL_USERNAME` only discovers playlists that are **public** on the server (the same set as `GET /users/{username}/playlists.json`). **Unlisted** playlists are not listed there; add their ids explicitly with `MUSIC_SOCIAL_PLAYLIST_ID`.

## Features

- Fetch tracks from music-social playlists over HTTPS (JSON API)
- Supply a **username** (all public playlists), **playlist id(s)**, or both (merged and deduplicated)
- Use track metadata from music-social (title, artist, album, duration; MusicBrainz ISRC/MBID when present)
- Match source tracks to your Plex music library [using the same rules as before](#matching-order-and-rules)
- Create or update Plex playlists; playlist summary includes a line like `synced from music-social: <url>`
- **Stateless** — no on-disk sync state; safe for ephemeral containers and cron without volumes
- **Playlist change preview** — before rewriting a Plex playlist, prints a git-style diff (adds / removals / substitutions) comparing current Plex items to the desired list under **SUMMARY**; then sync runs as before

### Playlist change preview

After matching tracks to your library, Plexify fetches the existing playlist’s items (if the playlist already exists), compares **ordered** `ratingKey` lists with an LCS-based diff, and prints a **PLAYLIST CHANGES** subsection inside **SUMMARY** (before **MISSING TRACKS SUMMARY** when there are gaps):

- Green `+`: tracks to add (music-social source line + matched Plex line + confidence)
- Red `-`: tracks to remove (Plex line only)
- Yellow `~`: substitution when a delete+insert pair is coalesced (previous Plex track → new source + new Plex + confidence)

Colors apply when stdout is a terminal. Set [`NO_COLOR`](https://no-color.org/) to force plain text. New playlists show an add-only diff (every matched track as `+`) before the playlist is created.

Because Plexify is stateless, it cannot highlight “same Plex track, different confidence vs last run”; yellow reflects a **different library track** at that edit (or a paired remove/add in order).

### If the wrong Plex playlist updates (or yours never changes)

Plexify is **authoritative** for each music-social playlist: it **creates** a Plex playlist if none matches the title, or **replaces** the contents of the matching one (clear + re-add). It does not require a manual Plex playlist id.

- **Dry-run:** `PLEXIFY_DRY_RUN=true` (or `-dry-run`) only prints the diff; it does not clear or add tracks on Plex.
- **Title match:** The Plex target is the playlist whose **title** equals the music-social playlist name (leading/trailing spaces ignored). If you have **two** Plex playlists with the same name, the first one returned by the server is updated—remove or rename the duplicate so only one matches.
- **Playlist listing:** Plex’s `GET /playlists` response is paginated; Plexify loads all pages so an older playlist is not missed when resolving by title.

## Prerequisites

- A reachable **music-social** deployment and its HTTPS base URL
- Plex Media Server with a music library and an `X-Plex-Token`

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

### 2. music-social URL

Set `MUSIC_SOCIAL_URL` to the **origin** of your instance (scheme + host, optional path prefix), for example `https://music.example.com`. Plexify calls:

- `GET {MUSIC_SOCIAL_URL}/users/{username}/playlists.json`
- `GET {MUSIC_SOCIAL_URL}/playlist/{id}.json`

No API token is required for these read-only endpoints on a typical music-social deployment.

**Docker example:**

```bash
docker run --rm \
  -e MUSIC_SOCIAL_URL='https://music.example.com' \
  -e MUSIC_SOCIAL_USERNAME='your_user' \
  -e PLEX_URL='http://plex:32400' \
  -e PLEX_TOKEN='your_token' \
  -e PLEX_LIBRARY_SECTION_ID='1' \
  ghcr.io/grrywlsn/plexify:latest
```

See [4. Configuration](#4-configuration) for all variables.

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

```env
# music-social (required)
MUSIC_SOCIAL_URL=https://your-music-social.example.com

# Plex (required)
PLEX_URL=http://your_plex_server:32400
PLEX_TOKEN=your_plex_token_here
PLEX_LIBRARY_SECTION_ID=your_music_library_section_id

# Playlists: at least one of USERNAME or PLAYLIST_ID is required

# Option 1: all public playlists for this account username
MUSIC_SOCIAL_USERNAME=your_username

# Option 2: comma-separated playlist ids from music-social
MUSIC_SOCIAL_PLAYLIST_ID=pl_abc123,pl_def456

# Optional: exclude ids when using USERNAME (or when merging lists)
MUSIC_SOCIAL_PLAYLIST_EXCLUDED_ID=pl_skip_this

# Optional Plex server id (auto-discovered if unset)
PLEX_SERVER_ID=

# Optional: Plex HTTPS — by default TLS certificate verification is skipped (LAN/self-signed). For strict verification:
# PLEX_VERIFY_TLS=true
# (Legacy: PLEX_INSECURE_SKIP_VERIFY=false forces verify; =true skips verify.)

# Optional: parallel Plex track lookups while matching (1 = sequential, max 32)
# PLEX_MATCH_CONCURRENCY=4

# Optional: cap Plex HTTP traffic (requests per second; default 4, 0 = unlimited)
# PLEX_MAX_REQUESTS_PER_SECOND=8
# (All Plex HTTP calls share one limiter, so high PLEX_MATCH_CONCURRENCY mostly queues on the limit.)

# Optional: dry-run — match and print playlist diff; do not create/clear/add on Plex
# PLEXIFY_DRY_RUN=true

# Optional: fast search — skip full-library scan; use Plex /search only (may miss hard matches)
# PLEXIFY_FAST_SEARCH=true
# (alias: PLEX_SKIP_FULL_LIBRARY_SEARCH=true)

# Optional: exact-matches-only — only the raw title/artist search strategy (no brackets/featuring/etc.); no full-library scan. Use with dry-run to spot metadata or library gaps quickly.
# PLEXIFY_EXACT_MATCHES_ONLY=true
```

Environment variables, a `.env` file, or flags (same names, e.g. `-MUSIC_SOCIAL_URL=...`) are all supported.

Additional CLI-only flags:

- `-dry-run` — same as `PLEXIFY_DRY_RUN=true`
- `-plex-match-concurrency=N` — overrides `PLEX_MATCH_CONCURRENCY` (1–32)
- `-plex-insecure-tls` — same as `PLEX_INSECURE_SKIP_VERIFY=true` (usually redundant; skipping verify is already the default)
- `-plex-verify-tls` — same as `PLEX_VERIFY_TLS=true` (enable certificate verification for Plex HTTPS)
- `-plex-fast-search` — same as `PLEXIFY_FAST_SEARCH=true` (no `/all` fallback)
- `-exact-matches-only` — same as `PLEXIFY_EXACT_MATCHES_ONLY=true` (first search strategy only; no `/all`)
- `-plex-max-rps=N` — overrides `PLEX_MAX_REQUESTS_PER_SECOND` (`0` = unlimited)

```bash
./plexify \
  -MUSIC_SOCIAL_URL=https://music.example.com \
  -MUSIC_SOCIAL_PLAYLIST_ID=pl_abc123 \
  -PLEX_URL=http://your_plex_server:32400 \
  -PLEX_TOKEN=your_plex_token_here \
  -PLEX_LIBRARY_SECTION_ID=6
```

### 5. Finding playlists

Playlist ids are the same as in the music-social UI URL: `https://your-instance/playlist/{id}`.

```env
MUSIC_SOCIAL_PLAYLIST_ID=pl_single

# Multiple playlists
MUSIC_SOCIAL_PLAYLIST_ID=pl_one,pl_two,pl_three
```

Or sync every **public** playlist for a user:

```env
MUSIC_SOCIAL_USERNAME=alice
```

Combine both: public playlists for `alice` plus extra ids, deduplicated:

```env
MUSIC_SOCIAL_USERNAME=alice
MUSIC_SOCIAL_PLAYLIST_ID=pl_extra_only_in_ids
```

#### Excluding playlists

```env
MUSIC_SOCIAL_PLAYLIST_EXCLUDED_ID=pl_no_sync,pl_also_skip
```

> [!IMPORTANT]
> Each source playlist becomes a Plex playlist with the **same title**. If a Plex playlist with that title already exists, it is **replaced** with the matched tracks from the source (existing items not in the source are removed).

#### Playlist artwork

music-social’s playlist JSON does not expose a cover URL, so Plexify usually **does not** set a Plex playlist poster.

## Results

### Example Output

```
Playlist: my favourite songs
Description: what I'm listening to right now
Total tracks: 62
Owner: alice

Songs in playlist (62 total):
================================================================================
  1. Bad Bunny - ALAMBRE PúA (ALAMBRE PúA)
  2. Mae Stephens - Tiny Voice (Tiny Voice)
  3. Georgia - Wanna Play (Wanna Play)
  ...
Successfully fetched 62 songs from music-social playlist

================================================================================
MATCHING SONGS TO PLEX LIBRARY
================================================================================
Matching song 1/62: Bad Bunny - ALAMBRE PúA
Matching song 2/62: Mae Stephens - Tiny Voice
...

================================================================================
MATCHING RESULTS
================================================================================
  1. Bad Bunny - ALAMBRE PúA: 🔍 Title/Artist match (Plex: Bad Bunny - ALAMBRE PúA)
  2. Mae Stephens - Tiny Voice: 🔍 Title/Artist match (Plex: Mae Stephens - Tiny Voice)
  3. Georgia - Wanna Play: ❌ No match
  ...

================================================================================
SUMMARY
================================================================================
Total songs: 62
Title/Artist matches: 57 (91.9%)
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
     ISRC: (not available)

  2. Some Artist - Some Song
     ISRC: USRC12345678
     MusicBrainz ID: 12345678-1234-1234-1234-123456789012 (https://musicbrainz.org/recording/12345678-1234-1234-1234-123456789012)

  3. Another Artist - Another Song
     ISRC: (not available)
```

**Note:** The missing-tracks section lists ISRC when known and a MusicBrainz recording link when music-social supplied a MBID for that track.

## Matching Order and Rules

### 1. **Exact Title/Artist Match** (First Priority)

**When it applies:** All songs (highest priority for reliability)
**What it does:** Searches using the exact title and artist from the source track without any modifications
**Rules:**

- Tries combined search: `"Song Title Artist Name"`
- Falls back to title-only search: `"Song Title"`
- Falls back to artist-only search: `"Artist Name"`
- Returns immediately if exact match is found (most reliable)
- **Comma-separated artists (music-social):** When the source lists several names in one `artist` field (e.g. `Le Youth, Forester, Robertson`), Plexify tries the **first** name first for Plex lookups—aligned with typical single-artist Plex metadata—then runs the same pipeline again with the **full** string if nothing matched (fallback for legitimate commas in a band name).

### 2. **Single Quote Handling** (Second Priority)

**When it applies:** Songs with apostrophes or single quotes in the title or artist name
**What it does:** Tries different variations of the quote characters to handle encoding differences
**Rules:**

- Original title with quotes: `"Don't Stop Believin'"`
- Remove all quotes: `"Dont Stop Believin"`
- Replace with backtick: `"Don`t Stop Believin``"`
- Replace with prime symbol: `"Don′t Stop Believin′"`
- Replace with different quote: `"Don't Stop Believin'"`
- Expand contractions: `"Do not Stop Believin"`, `"Do not Stop Believin is"`

### 3. **Bracket Removal** (Third Priority)

**When it applies:** Songs with text in parentheses, square brackets, or curly brackets
**What it does:** Removes bracketed content and searches again
**Rules:**

- `"Song Title (feat. Artist)"` → `"Song Title"`
- `"Song Title [Remix]"` → `"Song Title"`
- `"Song Title {Live}"` → `"Song Title"`
- Cleans up extra spaces after removal

### 4. **Featuring Removal** (Fourth Priority)

**When it applies:** Songs with featuring/feat/ft text in the title
**What it does:** Removes featuring information and searches again
**Rules:**

- `"Song Title featuring Artist"` → `"Song Title"`
- `"Song Title feat. Artist"` → `"Song Title"`
- `"Song Title ft. Artist"` → `"Song Title"`

### 5. **Title Normalization** (Fifth Priority)

**When it applies:** Songs with dashes in the title that might be formatted differently in Plex
**What it does:** Converts dash-separated titles to parentheses format for better matching
**Rules:**

- `"Mood Ring - Pride Remix"` → `"Mood Ring (Pride Remix)"`
- `"Song Title - Extended - Live"` → `"Song Title (Extended) (Live)"`
- Handles multiple dashes by converting each to separate parentheses

### 6. **"With" Removal** (Sixth Priority)

**When it applies:** Songs with "with" in the title that might be formatted differently
**What it does:** Removes "with" and any text after it from the title
**Rules:**

- `"Song Title with Artist"` → `"Song Title"`
- `"With Artist - Song Title"` → `"Song Title"`
- Cleans up trailing dashes and spaces after removal

### 7. **Common Suffixes Removal** (Seventh Priority)

**When it applies:** Songs with common suffixes that might not be present in Plex
**What it does:** Removes common track suffixes and variations
**Rules:**

- `"Song Title - Bonus Track"` → `"Song Title"`
- `"Song Title - Remix"` → `"Song Title"`
- `"Song Title - Extended"` → `"Song Title"`
- `"Song Title - Radio Edit"` → `"Song Title"`
- `"Song Title - Live"` → `"Song Title"`
- `"Song Title - Acoustic"` → `"Song Title"`
- `"Song Title - Remastered"` → `"Song Title"`
- `"Song Title (Bonus Track)"` → `"Song Title"`
- `"Song Title (Remix)"` → `"Song Title"`
- `"Song Title (Remastered)"` → `"Song Title"`
- `"Song Title - 2018 Remastered"` → `"Song Title"`
- `"Song Title (2018 Remastered)"` → `"Song Title"`
- **Soundtrack suffixes:**
  - `"Song Title - From the Motion Picture "Very Famous Movie""` → `"Song Title"`
  - `"Song Title - From the Film "Very Famous Movie""` → `"Song Title"`
  - `"Song Title - From the Movie "Very Famous Movie""` → `"Song Title"`
  - `"Song Title - Love Theme from "Very Famous Movie""` → `"Song Title"`
  - `"Song Title (From the Motion Picture "Very Famous Movie")"` → `"Song Title"`
  - `"Song Title (Love Theme from "Very Famous Movie")"` → `"Song Title"`
  - `"Song Title - Soundtrack Version"` → `"Song Title"`
  - `"Song Title - Film Version"` → `"Song Title"`
  - `"Song Title - Movie Version"` → `"Song Title"`
- And many more variations (clean, explicit, demo, instrumental, etc.)

### 8. **Full Library Search** (Eighth Priority - Fallback)

**When it applies:** When all other matching strategies fail
**What it does:** Searches through the entire music library to find potential matches
**Rules:**

- Uses the most comprehensive search method
- Applies similarity scoring to find the best match
- Used as a last resort when other methods don't find matches

## Matching issues

If you run into issues where plexify will not match a song that you know is in your Plex library, [please raise an issue in this repo](https://github.com/grrywlsn/plexify/issues), and include:

- the artist name and track name from music-social
- the artist name and track name from your Plex

Please copy/paste them **exactly** as they appear in each source, so that the matching can be tested.

### Debug mode

You can enable debug logs (`DEBUG=true`) to see the rules being evaluated and how they are scored.

It should help make it clear why one song wins over another, and can be included when raising the issues above.

```
2025/08/04 08:52:40 ⏭️  FindBestMatch: skipping 'Can’t Get You Out of My Head (Deluxe’s Dirty Dub)' by 'Kylie Minogue' (score: 0.361, current best: 0.385)
2025/08/04 08:52:40 🔍 FindBestMatch: 'Out Of My Head' by 'Loote' -> 'Can't Get You Out of My Head' by 'Kylie Minogue'
2025/08/04 08:52:40    Original title similarity: 0.500 ('out of my head' vs 'can't get you out of my head')
2025/08/04 08:52:40    Original artist similarity: 0.115 ('loote' vs 'kylie minogue')
2025/08/04 08:52:40    Clean title similarity: 0.500 ('out of my head' vs 'can't get you out of my head')
2025/08/04 08:52:40    Featuring-removed title similarity: 0.500 ('out of my head' vs 'can't get you out of my head')
2025/08/04 08:52:40    Normalized title similarity: 0.500 ('out of my head' vs 'can't get you out of my head')
2025/08/04 08:52:40    'With'-removed title similarity: 0.500 ('out of my head' vs 'can't get you out of my head')
2025/08/04 08:52:40    Suffix-removed title similarity: 0.500 ('out of my head' vs 'can't get you out of my head')
2025/08/04 08:52:40    Punctuation-normalized title similarity: 0.500 ('out of my head' vs 'can't get you out of my head')
2025/08/04 08:52:40    Final title similarity: 0.500
2025/08/04 08:52:40    Final artist similarity: 0.115
2025/08/04 08:52:40    Combined score: 0.385 (0.500 * 0.7 + 0.115 * 0.3)
2025/08/04 08:52:40 ⏭️  FindBestMatch: skipping 'Can't Get You Out of My Head' by 'Kylie Minogue' (score: 0.385, current best: 0.385)
2025/08/04 08:52:40 🔍 FindBestMatch: 'Out Of My Head' by 'Loote' -> 'Out of My Head' by 'Various Artists'
2025/08/04 08:52:40    Original title similarity: 1.000 ('out of my head' vs 'out of my head')
2025/08/04 08:52:40    Original artist similarity: 0.100 ('loote' vs 'various artists')
2025/08/04 08:52:40    Clean title similarity: 1.000 ('out of my head' vs 'out of my head')
2025/08/04 08:52:40    Featuring-removed title similarity: 1.000 ('out of my head' vs 'out of my head')
2025/08/04 08:52:40    Normalized title similarity: 1.000 ('out of my head' vs 'out of my head')
2025/08/04 08:52:40    'With'-removed title similarity: 1.000 ('out of my head' vs 'out of my head')
2025/08/04 08:52:40    Suffix-removed title similarity: 1.000 ('out of my head' vs 'out of my head')
2025/08/04 08:52:40    Punctuation-normalized title similarity: 1.000 ('out of my head' vs 'out of my head')
2025/08/04 08:52:40    Final title similarity: 1.000
2025/08/04 08:52:40    Final artist similarity: 0.100
2025/08/04 08:52:40    Combined score: 0.730 (1.000 * 0.7 + 0.100 * 0.3)
2025/08/04 08:52:40 🎵 FindBestMatch: allowing 'Various Artists' compilation match 'Out of My Head' by 'Various Artists' (title: 1.000 > 0.9, artist: 0.100 < 0.3 but is Various Artists)
2025/08/04 08:52:40 🎵 FindBestMatch: allowing 'Various Artists' compilation match 'Out of My Head' by 'Various Artists' (title: 1.000 > 0.7, artist: 0.100 < 0.2 but is Various Artists)
2025/08/04 08:52:40 📈 FindBestMatch: new best match 'Out of My Head' by 'Various Artists' (score: 0.730 > 0.385, title: 1.000, artist: 0.100)
2025/08/04 08:52:40 ✅ FindBestMatch: FINAL RESULT - returning match 'Out of My Head' by 'Various Artists' (score: 0.730 >= 0.700) for search 'Out Of My Head' by 'Loote'
2025/08/04 08:52:40 ✅ searchByTitle: found match 'Out of My Head' by 'Various Artists'
2025/08/04 08:52:40 ✅ SearchTrack: found match 'Out of My Head' by 'Various Artists' using exact title/artist
```