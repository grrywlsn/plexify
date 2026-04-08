package plex

import (
	"encoding/xml"
	"io"
	"net/http"
)

// decodePlexResponseXML reads the full response body then unmarshals XML.
// Prefer this over streaming xml.NewDecoder(resp.Body) for Plex API calls whose
// payloads are modest (search, server info): it avoids subtle failures when the
// connection or context lifecycle interacts with a streaming decoder.
func decodePlexResponseXML(resp *http.Response, v any) error {
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return xml.Unmarshal(raw, v)
}
