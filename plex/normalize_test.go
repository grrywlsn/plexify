package plex

import "testing"

func TestRemoveColorsShowSuffix(t *testing.T) {
	c := &Client{}
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "en dash like MusicBrainz",
			in:   "Pinterest (Portuguese) – A COLORS SHOW",
			want: "Pinterest (Portuguese)",
		},
		{
			name: "ASCII hyphen",
			in:   "Pinterest (Portuguese) - A COLORS SHOW",
			want: "Pinterest (Portuguese)",
		},
		{
			name: "em dash",
			in:   "Some Track — A COLORS SHOW",
			want: "Some Track",
		},
		{
			name: "case insensitive suffix",
			in:   "Track Name - a colors show",
			want: "Track Name",
		},
		{
			name: "no suffix unchanged",
			in:   "Pinterest (Portuguese)",
			want: "Pinterest (Portuguese)",
		},
		{
			name: "substring that is not suffix unchanged",
			in:   "A COLORS SHOW opener",
			want: "A COLORS SHOW opener",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.removeColorsShowSuffix(tt.in)
			if got != tt.want {
				t.Fatalf("removeColorsShowSuffix(%q) = %q; want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestRemoveCommonSuffixes_colorsShowIntegrated(t *testing.T) {
	c := &Client{}
	// RemoveCommonSuffixes applies removeColorsShowSuffix first; result should match Plex-style title.
	got := c.RemoveCommonSuffixes("Pinterest (Portuguese) – A COLORS SHOW")
	want := "Pinterest (Portuguese)"
	if got != want {
		t.Fatalf("RemoveCommonSuffixes(...) = %q; want %q", got, want)
	}
}
