package app

import (
	"crypto/sha256"
	"encoding/json"
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
)

func normalizeList(input radarport.ListInput) (radarport.ListInput, error) {
	if input.Status == "" {
		input.Status = radarport.StatusFilterAll
	}
	if input.Sort == "" {
		input.Sort = radarport.SortUpdatedDesc
	}
	if input.Limit == 0 {
		input.Limit = radarport.DefaultLimit
	}
	if !input.Status.Valid() {
		return radarport.ListInput{}, radarport.Invalid("status", "unsupported_value")
	}
	if !input.Sort.Valid() {
		return radarport.ListInput{}, radarport.Invalid("sort", "unsupported_value")
	}
	if input.Limit < 1 || input.Limit > radarport.MaximumLimit {
		return radarport.ListInput{}, radarport.Invalid("limit", "out_of_range")
	}
	if input.Offset < 0 || input.Offset > radarport.MaximumOffset {
		return radarport.ListInput{}, radarport.Invalid("offset", "out_of_range")
	}
	return input, nil
}

func normalizeCreate(command radarport.CreateCommand) (radarport.CreateCommand, error) {
	command.Name = strings.TrimSpace(command.Name)
	command.Title = strings.TrimSpace(command.Title)
	command.DestinationURL = strings.TrimSpace(command.DestinationURL)
	if command.ExpectedVersion != 0 {
		return radarport.CreateCommand{}, radarport.Invalid("expected_version", "must_be_zero")
	}
	if command.ActorID < 1 {
		return radarport.CreateCommand{}, radarport.Invalid("actor", "required")
	}
	if !validIdempotencyKey(command.IdempotencyKey) {
		return radarport.CreateCommand{}, radarport.Invalid("idempotency_key", "invalid")
	}
	if err := validateName(command.Name); err != nil {
		return radarport.CreateCommand{}, err
	}
	if err := validateTitle(command.Title); err != nil {
		return radarport.CreateCommand{}, err
	}
	if err := ValidateDestinationURL(command.DestinationURL); err != nil {
		return radarport.CreateCommand{}, err
	}
	if err := validateNullableLocalID("cover_image_id", command.CoverImageID); err != nil {
		return radarport.CreateCommand{}, err
	}
	if err := validateNullableLocalID("attachment_id", command.AttachmentID); err != nil {
		return radarport.CreateCommand{}, err
	}
	command.CoverImageID = cloneID(command.CoverImageID)
	command.AttachmentID = cloneID(command.AttachmentID)
	return command, nil
}

func normalizeUpdate(command radarport.UpdateCommand) (radarport.UpdateCommand, error) {
	if command.LinkID < 1 {
		return radarport.UpdateCommand{}, radarport.Invalid("link_id", "invalid")
	}
	if command.ExpectedVersion < 1 {
		return radarport.UpdateCommand{}, radarport.Invalid("expected_version", "out_of_range")
	}
	if command.ActorID < 1 {
		return radarport.UpdateCommand{}, radarport.Invalid("actor", "required")
	}
	if !validIdempotencyKey(command.IdempotencyKey) {
		return radarport.UpdateCommand{}, radarport.Invalid("idempotency_key", "invalid")
	}
	if !command.Name.Set && !command.Title.Set && !command.DestinationURL.Set && !command.CoverImageID.Set && !command.AttachmentID.Set {
		return radarport.UpdateCommand{}, radarport.Invalid("body", "no_mutable_fields")
	}
	if command.Name.Set {
		command.Name.Value = strings.TrimSpace(command.Name.Value)
		if err := validateName(command.Name.Value); err != nil {
			return radarport.UpdateCommand{}, err
		}
	}
	if command.Title.Set {
		command.Title.Value = strings.TrimSpace(command.Title.Value)
		if err := validateTitle(command.Title.Value); err != nil {
			return radarport.UpdateCommand{}, err
		}
	}
	if command.DestinationURL.Set {
		command.DestinationURL.Value = strings.TrimSpace(command.DestinationURL.Value)
		if err := ValidateDestinationURL(command.DestinationURL.Value); err != nil {
			return radarport.UpdateCommand{}, err
		}
	}
	if command.CoverImageID.Set {
		if err := validateNullableLocalID("cover_image_id", command.CoverImageID.Value); err != nil {
			return radarport.UpdateCommand{}, err
		}
		command.CoverImageID.Value = cloneID(command.CoverImageID.Value)
	}
	if command.AttachmentID.Set {
		if err := validateNullableLocalID("attachment_id", command.AttachmentID.Value); err != nil {
			return radarport.UpdateCommand{}, err
		}
		command.AttachmentID.Value = cloneID(command.AttachmentID.Value)
	}
	return command, nil
}

func normalizeStatus(command radarport.SetStatusCommand) (radarport.SetStatusCommand, error) {
	if command.LinkID < 1 {
		return radarport.SetStatusCommand{}, radarport.Invalid("link_id", "invalid")
	}
	if command.ExpectedVersion < 1 {
		return radarport.SetStatusCommand{}, radarport.Invalid("expected_version", "out_of_range")
	}
	if command.Target != radarport.StatusEnabled && command.Target != radarport.StatusDisabled {
		return radarport.SetStatusCommand{}, radarport.Invalid("status", "unsupported_value")
	}
	if command.ActorID < 1 {
		return radarport.SetStatusCommand{}, radarport.Invalid("actor", "required")
	}
	if !validIdempotencyKey(command.IdempotencyKey) {
		return radarport.SetStatusCommand{}, radarport.Invalid("idempotency_key", "invalid")
	}
	return command, nil
}

func validateName(value string) error {
	if value == "" {
		return radarport.Invalid("name", "required")
	}
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > radarport.MaximumNameRunes || containsControl(value) {
		return radarport.Invalid("name", "invalid")
	}
	return nil
}

func validateTitle(value string) error {
	if value == "" {
		return radarport.Invalid("title", "required")
	}
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > radarport.MaximumTitleRunes || containsControl(value) {
		return radarport.Invalid("title", "invalid")
	}
	return nil
}

func validateNullableLocalID(field string, value *int64) error {
	if value != nil && *value < 1 {
		return radarport.Invalid(field, "must_be_positive")
	}
	return nil
}

func validIdempotencyKey(value string) bool {
	if len(value) < radarport.MinimumIdempotencyKeyBytes || len(value) > radarport.MaximumIdempotencyKeyBytes || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range []byte(value) {
		if character < 0x21 || character > 0x7e || character == ',' {
			return false
		}
	}
	return true
}

func containsControl(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}

// ValidateDestinationURL performs syntax-only validation. It deliberately does
// not resolve DNS or issue a request. Literal IP addresses are rejected in full,
// which is stricter than merely rejecting their private ranges.
func ValidateDestinationURL(value string) error {
	if value == "" || len(value) > radarport.MaximumURLBytes || strings.TrimSpace(value) != value || !utf8.ValidString(value) || containsControl(value) || strings.Contains(value, `\`) {
		return radarport.Invalid("destination_url", "invalid")
	}
	parsed, err := url.Parse(value)
	if err != nil || !parsed.IsAbs() || parsed.Scheme != "https" || parsed.Opaque != "" || parsed.Host == "" || parsed.User != nil {
		return radarport.Invalid("destination_url", "https_absolute_required")
	}
	if containsControl(parsed.Path) || containsControl(parsed.Fragment) || strings.Contains(parsed.Path, `\`) {
		return radarport.Invalid("destination_url", "invalid")
	}
	if decodedQuery, decodeErr := url.QueryUnescape(parsed.RawQuery); decodeErr != nil || containsControl(decodedQuery) {
		return radarport.Invalid("destination_url", "invalid")
	}
	if parsed.Hostname() == "" || strings.ContainsAny(parsed.Host, "%@") {
		return radarport.Invalid("destination_url", "invalid_host")
	}
	if strings.HasSuffix(parsed.Host, ":") {
		return radarport.Invalid("destination_url", "invalid_port")
	}
	if port := parsed.Port(); port != "" {
		portNumber, parseErr := strconv.ParseUint(port, 10, 16)
		if parseErr != nil || portNumber == 0 {
			return radarport.Invalid("destination_url", "invalid_port")
		}
	}

	host := strings.ToLower(parsed.Hostname())
	if strings.HasSuffix(host, ".") || host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return radarport.Invalid("destination_url", "invalid_host")
	}
	if net.ParseIP(host) != nil || numericHostAlias(host) {
		return radarport.Invalid("destination_url", "ip_literal_forbidden")
	}
	if privateLookingHostname(host) || !publicDNSName(host) {
		return radarport.Invalid("destination_url", "public_hostname_required")
	}
	return nil
}

func numericHostAlias(host string) bool {
	if _, err := strconv.ParseUint(host, 0, 64); err == nil {
		return true
	}
	parts := strings.Split(host, ".")
	if len(parts) < 2 || len(parts) > 4 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		if _, err := strconv.ParseUint(part, 0, 32); err != nil {
			return false
		}
	}
	return true
}

func privateLookingHostname(host string) bool {
	for _, suffix := range []string{".local", ".internal", ".home", ".lan", ".intranet", ".test", ".invalid"} {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

func publicDNSName(host string) bool {
	if len(host) > 253 || !strings.Contains(host, ".") {
		return false
	}
	labels := strings.Split(host, ".")
	for _, label := range labels {
		if len(label) < 1 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if !(character >= 'a' && character <= 'z') && !(character >= '0' && character <= '9') && character != '-' {
				return false
			}
		}
	}
	topLevel := labels[len(labels)-1]
	allNumeric := true
	for _, character := range topLevel {
		if character < '0' || character > '9' {
			allNumeric = false
			break
		}
	}
	return !allNumeric
}

func validateStoredLink(link radarport.Link) bool {
	if link.LinkID < 1 || !validPublicCode(link.PublicCode) || link.Version < 1 || link.CreatedBy < 1 || link.UpdatedBy < 1 || link.CreatedAt.IsZero() || link.UpdatedAt.IsZero() || link.UpdatedAt.Before(link.CreatedAt) || !link.Status.Valid() {
		return false
	}
	if validateName(link.Name) != nil || validateTitle(link.Title) != nil || ValidateDestinationURL(link.DestinationURL) != nil || validateNullableLocalID("cover_image_id", link.CoverImageID) != nil || validateNullableLocalID("attachment_id", link.AttachmentID) != nil {
		return false
	}
	return true
}

func validPublicCode(code string) bool {
	if !strings.HasPrefix(code, "rd_") || len(code) != 25 {
		return false
	}
	for _, character := range code[3:] {
		if !(character >= 'a' && character <= 'z') && !(character >= 'A' && character <= 'Z') && !(character >= '0' && character <= '9') && character != '-' && character != '_' {
			return false
		}
	}
	return true
}

func cloneID(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func equivalentID(left, right *int64) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func canonicalDigest(value any) ([32]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return [32]byte{}, radarport.ErrInvalidArgument
	}
	return sha256.Sum256(raw), nil
}
