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
			if art["monitored"] != true {
				t.Error("expected artist monitored true")
			}
			aopts, _ := art["addOptions"].(map[string]interface{})
			if aopts == nil || aopts["monitor"] != "all" {
				t.Errorf("expected artist addOptions.monitor all, got %v", aopts)
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
	var monitorBody map[string]interface{}
	var putAlbumBody map[string]interface{}
	var putArtistBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/album" && r.URL.Query().Get("foreignAlbumId") == mbid:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"id":1,"foreignAlbumId":"` + mbid + `"}]`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/album/monitor":
			_ = json.NewDecoder(r.Body).Decode(&monitorBody)
			w.WriteHeader(http.StatusAccepted)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/album/1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":               1.0,
				"foreignAlbumId":   mbid,
				"monitored":        false,
				"title":            "Existing",
				"artist":           map[string]interface{}{"id": 10.0, "monitored": false},
				"releases":         []interface{}{map[string]interface{}{"id": 5.0, "monitored": false, "title": "R1"}},
				"albumReleases":    []interface{}{},
				"anyReleaseOk":     true,
				"artistId":         10.0,
				"artistMetadataId": 99.0,
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/album/1":
			_ = json.NewDecoder(r.Body).Decode(&putAlbumBody)
			w.WriteHeader(http.StatusAccepted)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/artist/10":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id": 10.0, "monitored": false, "artistName": "Test Artist",
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/artist/10":
			_ = json.NewDecoder(r.Body).Decode(&putArtistBody)
			w.WriteHeader(http.StatusAccepted)
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
	if !res.AlreadyPresent || res.Added || !res.EnsuredMonitored {
		t.Fatalf("got %#v", res)
	}
	if monitorBody == nil {
		t.Fatal("expected PUT /album/monitor")
	}
	ids, _ := monitorBody["albumIds"].([]interface{})
	if len(ids) != 1 || int(ids[0].(float64)) != 1 {
		t.Fatalf("albumIds: %v", monitorBody["albumIds"])
	}
	if monitorBody["monitored"] != true {
		t.Fatalf("monitored: %v", monitorBody["monitored"])
	}
	if putAlbumBody == nil || putAlbumBody["monitored"] != true {
		t.Fatalf("PUT album body monitored: %v", putAlbumBody)
	}
	artPut, _ := putAlbumBody["artist"].(map[string]interface{})
	if artPut == nil || artPut["monitored"] != true {
		t.Fatalf("PUT album nested artist monitored: %v", artPut)
	}
	rels, _ := putAlbumBody["releases"].([]interface{})
	if len(rels) != 1 {
		t.Fatalf("releases: %v", putAlbumBody["releases"])
	}
	rel, _ := rels[0].(map[string]interface{})
	if rel["monitored"] != true {
		t.Fatalf("release monitored: %v", rel["monitored"])
	}
	if putArtistBody == nil || putArtistBody["monitored"] != true {
		t.Fatalf("PUT artist monitored: %v", putArtistBody)
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

func TestAddReleaseGroupIfMissing_MonitorsArtistAndReleases(t *testing.T) {
	mbid := "33333333-3333-3333-3333-333333333333"
	lookup := []map[string]interface{}{
		{
			"title":          "RG Album",
			"foreignAlbumId": mbid,
			"releases": []interface{}{
				map[string]interface{}{"foreignReleaseId": "rel-1", "title": "Deluxe", "monitored": false},
			},
			"artist": map[string]interface{}{
				"foreignArtistId": "artist-mb-1",
				"monitored":       false,
				"addOptions": map[string]interface{}{
					"monitor": "none",
				},
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/album" && r.URL.Query().Get("foreignAlbumId") == mbid:
			_, _ = w.Write([]byte("[]"))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/rootfolder":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{{
				"path":                     "/music",
				"defaultQualityProfileId":  1,
				"defaultMetadataProfileId": 2,
			}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/album/lookup" && r.URL.Query().Get("term") == "lidarr:"+mbid:
			_ = json.NewEncoder(w).Encode(lookup)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/album":
			var got map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&got)
			if got["monitored"] != true {
				t.Errorf("album monitored: %v", got["monitored"])
			}
			art, _ := got["artist"].(map[string]interface{})
			if art["monitored"] != true {
				t.Errorf("artist monitored: %v", art["monitored"])
			}
			aopts, _ := art["addOptions"].(map[string]interface{})
			if aopts["monitor"] != "all" {
				t.Errorf("artist addOptions.monitor: %v", aopts["monitor"])
			}
			rels, _ := got["releases"].([]interface{})
			if len(rels) != 1 {
				t.Fatalf("releases len: %d", len(rels))
			}
			rel, _ := rels[0].(map[string]interface{})
			if rel["monitored"] != true {
				t.Errorf("release monitored: %v", rel["monitored"])
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
	_, err = c.AddReleaseGroupIfMissing(context.Background(), mbid)
	if err != nil {
		t.Fatal(err)
	}
}
