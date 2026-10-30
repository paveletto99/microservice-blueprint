package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"
)

// MaxTokenBytes bounds both JWT parsing and outbound token responses.
const MaxTokenBytes = 16 * 1024

// ValidateEndpoint requires a fixed HTTPS URL unless explicit isolated development HTTP is enabled.
func ValidateEndpoint(raw string, developmentHTTP bool) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || (u.Scheme != "https" && !(developmentHTTP && u.Scheme == "http")) {
		return errors.New("auth configuration: invalid endpoint or HTTPS required")
	}
	return nil
}

// Provider extensions are ignored, but duplicate fields, depth and byte limits still apply.
func decodeProviderJSON(reader io.Reader, limit int64, target any) error {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil || int64(len(data)) > limit {
		return errors.New("auth decoding: response exceeds limit or cannot be read")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := checkJSONNames(json.NewDecoder(bytes.NewReader(data)), 0); err != nil {
		return errors.New("auth decoding: duplicate fields or invalid JSON structure")
	}
	if err := decoder.Decode(target); err != nil {
		return errors.New("auth decoding: malformed JSON")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("auth decoding: trailing JSON")
	}
	return nil
}

func checkJSONNames(decoder *json.Decoder, depth int) error {
	if depth > 32 {
		return errors.New("JSON nesting exceeds limit")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	if delim == '{' {
		seen := make(map[string]bool)
		for decoder.More() {
			name, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := name.(string)
			if !ok || seen[key] {
				return errors.New("duplicate JSON field")
			}
			seen[key] = true
			if err := checkJSONNames(decoder, depth+1); err != nil {
				return err
			}
		}
	} else if delim == '[' {
		for decoder.More() {
			if err := checkJSONNames(decoder, depth+1); err != nil {
				return err
			}
		}
	} else {
		return errors.New("unexpected JSON delimiter")
	}
	_, err = decoder.Token()
	return err
}

func boundedHTTPClient(source *http.Client) *http.Client {
	client := &http.Client{}
	if source != nil {
		*client = *source
	}
	client.Timeout = 5 * time.Second
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if client.Transport == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.MaxConnsPerHost = 8
		transport.MaxIdleConnsPerHost = 8
		transport.ResponseHeaderTimeout = 3 * time.Second
		transport.TLSHandshakeTimeout = 3 * time.Second
		client.Transport = transport
	}
	return client
}
