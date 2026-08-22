package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	stdhttp "net/http"
	"reflect"
	"strings"
	"unicode/utf8"

	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
)

var (
	errMalformedJSON       = errors.New("malformed radar json")
	errUnsupportedMedia    = errors.New("unsupported radar media type")
	errRequestBodyTooLarge = errors.New("radar request body too large")
)

func decodeStrictJSON(request *stdhttp.Request, destination any) error {
	if request == nil || request.Body == nil || destination == nil {
		return errMalformedJSON
	}
	contentTypes := request.Header.Values("Content-Type")
	if len(contentTypes) != 1 {
		return errUnsupportedMedia
	}
	mediaType, parameters, err := mime.ParseMediaType(contentTypes[0])
	if err != nil || mediaType != "application/json" {
		return errUnsupportedMedia
	}
	for key, value := range parameters {
		if !strings.EqualFold(key, "charset") || !strings.EqualFold(value, "utf-8") {
			return errUnsupportedMedia
		}
	}
	if request.ContentLength > radarport.MaximumRequestBodyBytes {
		return errRequestBodyTooLarge
	}
	raw, err := io.ReadAll(io.LimitReader(request.Body, radarport.MaximumRequestBodyBytes+1))
	if err != nil {
		return errMalformedJSON
	}
	if int64(len(raw)) > radarport.MaximumRequestBodyBytes {
		return errRequestBodyTooLarge
	}
	if len(bytes.TrimSpace(raw)) == 0 || !utf8.Valid(raw) || validateTopLevelKeys(raw, destination) != nil {
		return errMalformedJSON
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(destination); err != nil {
		return errMalformedJSON
	}
	if err = decoder.Decode(&struct{}{}); err != io.EOF {
		return errMalformedJSON
	}
	return nil
}

func validateTopLevelKeys(raw []byte, destination any) error {
	allowed, err := exactJSONFields(destination)
	if err != nil {
		return errMalformedJSON
	}
	return rejectDuplicateOrUnknownTopLevelKeys(raw, allowed)
}

func exactJSONFields(destination any) (map[string]struct{}, error) {
	typeOf := reflect.TypeOf(destination)
	if typeOf == nil || typeOf.Kind() != reflect.Pointer || typeOf.Elem().Kind() != reflect.Struct {
		return nil, errMalformedJSON
	}
	typeOf = typeOf.Elem()
	allowed := make(map[string]struct{}, typeOf.NumField())
	for index := 0; index < typeOf.NumField(); index++ {
		field := typeOf.Field(index)
		if field.PkgPath != "" { // unexported
			continue
		}
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "" {
			name = field.Name
		}
		if name == "-" {
			continue
		}
		if _, duplicate := allowed[name]; duplicate {
			return nil, errMalformedJSON
		}
		allowed[name] = struct{}{}
	}
	return allowed, nil
}

func rejectDuplicateOrUnknownTopLevelKeys(raw []byte, allowed map[string]struct{}) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return errMalformedJSON
	}
	seen := make(map[string]struct{})
	for decoder.More() {
		token, tokenErr := decoder.Token()
		key, ok := token.(string)
		if tokenErr != nil || !ok {
			return errMalformedJSON
		}
		if _, known := allowed[key]; !known {
			return errMalformedJSON
		}
		if _, duplicate := seen[key]; duplicate {
			return errMalformedJSON
		}
		seen[key] = struct{}{}
		var discarded json.RawMessage
		if decoder.Decode(&discarded) != nil {
			return errMalformedJSON
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return errMalformedJSON
	}
	if err = decoder.Decode(&struct{}{}); err != io.EOF {
		return errMalformedJSON
	}
	return nil
}

type optionalString struct {
	Set   bool
	Value string
}

func (field *optionalString) UnmarshalJSON(raw []byte) error {
	if field == nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return errMalformedJSON
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return errMalformedJSON
	}
	field.Set = true
	field.Value = value
	return nil
}

type optionalNullableID struct {
	Set   bool
	Value *int64
}

func (field *optionalNullableID) UnmarshalJSON(raw []byte) error {
	if field == nil {
		return errMalformedJSON
	}
	field.Set = true
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		field.Value = nil
		return nil
	}
	var value int64
	if json.Unmarshal(raw, &value) != nil {
		return errMalformedJSON
	}
	field.Value = &value
	return nil
}
