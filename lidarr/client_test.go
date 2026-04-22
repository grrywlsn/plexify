package lidarr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grrywlsn/plexify/config"
)

func TestNewClientEmpty(t *testing.T) {
	_, err := NewClient(&config.LidarrConfig{URL: "http://127.0.0.1:1", Token: ""})
	if err == nil {
		t.Fatal("expected error for empty token")
	}
	_, err = NewClient(&config.LidarrConfig{URL: "", Token: "k"})
	if err == nil {
		t.Fatal("expected error for empty URL")
	}
}

func TestAddReleaseGroupIfMissing_Adds(t *testing.T) {
	mbid := "46336fb9-d9d6-4322-b044-d7f7ded5e5e2"
	lookup := []map[string]interface{}{
		{
			"title":          "Test Album",
			"foreignAlbumId": mbid,
			"artist":         map[string]interface{}{"foreignArtistId": "a1"},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/album" && r.URL.Query().Get("foreignAlbumId") == mbid:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("[]"))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/rootfolder":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{{
				"path":                     "/music",
				"defaultQualityProfileId":  1,
				"defaultMetadataProfileId": 2,
			}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/album/lookup":
			if r.URL.Query().Get("term") == "lidarr:"+mbid {
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(lookup)
				return
			}
			_, _ = w.Write([]byte("[]"))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/album":
			var got map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&got)
			ao, _ := got["addOptions"].(map[string]interface{})
			if ao == nil || ao["searchForNewAlbum"] != true {
				t.Error("expected addOptions.searchForNewAlbum true")
			}
			if got["monitored"] != true {
				t.Error("expected monitored true")
			}
			art, _ := got["artist"].(map[string]interface{})
			if art == nil {
				t.Fatal("expected artist in POST body")
			}
			if int(art["qualityProfileId"].(float64)) != 1 {
				t.Errorf("expected qualityProfileId 1, got %v", art["qualityProfileId"])
			}
			if int(art["metadataProfileId"].(float64)) != 2 {
				t.Errorf("expected metadataProfileId 2, got %v", art["metadataProfileId"])
			}
			if art["rootFolderPath"] != "/music" {
				t.Errorf("rootFolderPath: %v", art["rootFolderPath"])
			}
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	c, err := NewClient(&config.LidarrConfig{URL: srv.URL, Token: "key"})
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.AddReleaseGroupIfMissing(context.Background(), mbid)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Added || res.AlreadyPresent {
		t.Fatalf("got %#v", res)
	}
}

func TestAddReleaseGroupIfMissing_SkipWhenPresent(t *testing.T) {
	mbid := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/album" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"id":1}]`))
			return
		}
		t.Fatalf("unexpected %s", r.URL.Path)
	}))
	defer srv.Close()

	c, err := NewClient(&config.LidarrConfig{URL: srv.URL, Token: "key"})
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.AddReleaseGroupIfMissing(context.Background(), mbid)
	if err != nil {
		t.Fatal(err)
	}
	if !res.AlreadyPresent || res.Added {
		t.Fatalf("got %#v", res)
	}
}

func TestFetchAddDefaults_UsesProfileListsWhenRootHasNoDefaults(t *testing.T) {
	mbid := "22222222-2222-2222-2222-222222222222"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/album" && r.URL.Query().Get("foreignAlbumId") == mbid:
			_, _ = w.Write([]byte("[]"))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/rootfolder":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{{"path": "/tmp/music"}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/qualityprofile":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{{"id": 7}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/metadataprofile":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{{"id": 8}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/album/lookup" && r.URL.Query().Get("term") == "lidarr:"+mbid:
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"foreignAlbumId": mbid, "title": "Y", "artist": map[string]interface{}{}},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/album/lookup":
			_, _ = w.Write([]byte("[]"))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/album":
			var got map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&got)
			art, _ := got["artist"].(map[string]interface{})
			if int(art["qualityProfileId"].(float64)) != 7 || int(art["metadataProfileId"].(float64)) != 8 {
				t.Errorf("artist profiles: %v", art)
			}
			if art["rootFolderPath"] != "/tmp/music" {
				t.Errorf("rootFolderPath: %v", art["rootFolderPath"])
			}
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	c, err := NewClient(&config.LidarrConfig{URL: srv.URL, Token: "k"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.AddReleaseGroupIfMissing(context.Background(), mbid)
	if err != nil {
		t.Fatal(err)
	}
}

func TestAddReleaseGroupIfMissing_FallbackTerm(t *testing.T) {
	mbid := "11111111-1111-1111-1111-111111111111"
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/album" {
			_, _ = w.Write([]byte("[]"))
			return
		}
		if r.URL.Path == "/api/v1/rootfolder" {
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{{
				"path":                     "/m",
				"defaultQualityProfileId":  3,
				"defaultMetadataProfileId": 4,
			}})
			return
		}
		if r.URL.Path == "/api/v1/album/lookup" {
			term := r.URL.Query().Get("term")
			if term == "lidarr:"+mbid {
				_, _ = w.Write([]byte("[]"))
				return
			}
			if term == mbid {
				calls++
				_ = json.NewEncoder(w).Encode([]map[string]interface{}{
					{"foreignAlbumId": mbid, "title": "X"},
				})
				return
			}
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/album" {
			w.WriteHeader(http.StatusCreated)
			return
		}
		t.Fatalf("unexpected %s", r.URL.String())
	}))
	defer srv.Close()

	c, err := NewClient(&config.LidarrConfig{URL: srv.URL, Token: "k"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.AddReleaseGroupIfMissing(context.Background(), mbid)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("expected fallback term used once, got %d", calls)
	}
}
