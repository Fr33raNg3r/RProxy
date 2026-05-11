package handlers

import (
	"encoding/json"
	"io"
)

func jsonEncode(w io.Writer, v interface{}) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
