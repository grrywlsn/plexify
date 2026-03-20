package app

import "errors"

// ErrNoPlaylists is returned when no playlists are configured or discovered to process.
var ErrNoPlaylists = errors.New("no playlists to process")
