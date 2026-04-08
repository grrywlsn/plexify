package plex

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
)

// decodePlexResponseXML reads the full response body then unmarshals XML.
// Prefer this over streaming xml.NewDecoder(resp.Body): it avoids subtle failures when the
// connection or context lifecycle interacts with a streaming decoder (including large
// /library/sections/{id}/all responses if the request context cancels mid-read).
func decodePlexResponseXML(resp *http.Response, v any) error {
	if resp == nil {
		return fmt.Errorf("nil response")
	}
	if resp.Body == nil {
		return fmt.Errorf("nil response body")
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return xml.Unmarshal(raw, v)
}
