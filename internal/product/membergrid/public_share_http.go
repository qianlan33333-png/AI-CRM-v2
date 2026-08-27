package membergrid

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

const PublicShareSummaryPath = "/api/public/member-grid-shares/summary"

type PublicShareApplication interface {
	Summary(context.Context, string) (PublicShareSummary, error)
}

type PublicShareHandler struct{ application PublicShareApplication }

func NewPublicShareHandler(application PublicShareApplication) (*PublicShareHandler, error) {
	if nilDependency(application) {
		return nil, errors.New("member grid public share application is required")
	}
	return &PublicShareHandler{application: application}, nil
}

func (handler *PublicShareHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	setPublicShareHeaders(writer.Header())
	if handler == nil || nilDependency(handler.application) || request == nil {
		writePublicShareError(writer, http.StatusServiceUnavailable)
		return
	}
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		writePublicShareError(writer, http.StatusMethodNotAllowed)
		return
	}
	if request.URL == nil || request.URL.Path != PublicShareSummaryPath || request.URL.RawQuery != "" || request.URL.Fragment != "" {
		writePublicShareError(writer, http.StatusNotFound)
		return
	}
	var input struct {
		Token string `json:"token"`
	}
	decoder := json.NewDecoder(io.LimitReader(request.Body, 1025))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || strings.TrimSpace(input.Token) != input.Token || input.Token == "" || len(input.Token) > 512 {
		writePublicShareError(writer, http.StatusBadRequest)
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		writePublicShareError(writer, http.StatusBadRequest)
		return
	}
	result, err := handler.application.Summary(request.Context(), input.Token)
	if err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrInvalidExternalShareToken) {
			writePublicShareError(writer, http.StatusNotFound)
			return
		}
		writePublicShareError(writer, http.StatusServiceUnavailable)
		return
	}
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(writer).Encode(result)
}

func setPublicShareHeaders(header http.Header) {
	header.Set("Cache-Control", "private, no-store")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("X-Content-Type-Options", "nosniff")
}

func writePublicShareError(writer http.ResponseWriter, status int) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]string{"error": http.StatusText(status)})
}
