package provider

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const maxProviderResponseBytes = 256 << 10

var (
	ErrInvalidProviderConfig   = errors.New("invalid payment provider config")
	ErrInvalidProviderMaterial = errors.New("invalid payment provider material")
	ErrInvalidProviderResponse = errors.New("invalid payment provider response")
	ErrProviderUnavailable     = errors.New("payment provider unavailable")
)

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

func validHTTPSBase(value string) (*url.URL, bool) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return nil, false
	}
	parsed.Path = ""
	return parsed, true
}

func validHTTPSURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil && parsed.Fragment == ""
}

func readProviderResponse(response *http.Response) ([]byte, error) {
	if response == nil || response.Body == nil {
		return nil, ErrInvalidProviderResponse
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxProviderResponseBytes+1))
	if err != nil || len(body) > maxProviderResponseBytes {
		return nil, ErrInvalidProviderResponse
	}
	return body, nil
}

func providerDigest(domain string, values ...string) [32]byte {
	return sha256.Sum256([]byte(domain + "\x00" + strings.Join(values, "\x00")))
}

func digestHex(value [32]byte) string { return hex.EncodeToString(value[:]) }

func zeroDigest(value [32]byte) bool { return value == ([32]byte{}) }
