package legacyaudience

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/segment/dsl"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

const maximumSortOrder int32 = 1_000_000

func normalizeGroupName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > MaximumGroupNameRunes {
		return "", ErrInvalidInput
	}
	return name, nil
}

func normalizePackageName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > MaximumPackageNameRunes {
		return "", ErrInvalidInput
	}
	return name, nil
}

func normalizeSortOrder(value int32) (int32, error) {
	if value < 0 || value > maximumSortOrder {
		return 0, ErrInvalidInput
	}
	return value, nil
}

func normalizePagination(input ListPackagesInput) (ListPackagesInput, error) {
	if input.Limit == 0 {
		input.Limit = DefaultLimit
	}
	if input.Limit < 1 || input.Limit > MaximumLimit || input.Offset < 0 || input.Offset > MaximumOffset {
		return ListPackagesInput{}, ErrInvalidInput
	}
	if input.GroupID != nil && *input.GroupID <= 0 {
		return ListPackagesInput{}, ErrInvalidInput
	}
	input.GroupID = cloneInt64(input.GroupID)
	return input, nil
}

func validateWriteCommon(actor Actor, key string, expectedVersion int64, allowZeroVersion bool) error {
	if actor.AdminUserID <= 0 || !validIdempotencyKey(key) {
		return ErrInvalidInput
	}
	if allowZeroVersion {
		if expectedVersion != 0 {
			return ErrInvalidInput
		}
		return nil
	}
	if expectedVersion <= 0 {
		return ErrInvalidInput
	}
	return nil
}

func validIdempotencyKey(value string) bool {
	if len(value) < 16 || len(value) > 128 || strings.TrimSpace(value) != value {
		return false
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if character < 0x21 || character > 0x7e || character == ',' {
			return false
		}
	}
	return true
}

func canonicalDefinition(raw segmentport.Definition) (segmentport.Definition, error) {
	ast, err := dsl.Parse(raw)
	if err != nil {
		return nil, errors.Join(ErrInvalidInput, err)
	}
	canonical, err := ast.CanonicalJSON()
	if err != nil {
		return nil, errors.Join(ErrInvalidInput, err)
	}
	return segmentport.Definition(canonical), nil
}

func canonicalRefreshCron(mode segmentport.RefreshMode, refreshCron *string) (*string, error) {
	switch mode {
	case segmentport.RefreshModeManual:
		if refreshCron != nil {
			return nil, ErrInvalidInput
		}
		return nil, nil
	case segmentport.RefreshModeScheduled:
		if refreshCron == nil {
			return nil, ErrInvalidInput
		}
		canonical, err := canonicalCron(*refreshCron)
		if err != nil {
			return nil, ErrInvalidInput
		}
		return &canonical, nil
	default:
		return nil, ErrInvalidInput
	}
}

func canonicalCron(raw string) (string, error) {
	fields := strings.Fields(raw)
	if len(fields) != 5 {
		return "", ErrInvalidInput
	}
	ranges := [][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 6}}
	canonical := make([]string, len(fields))
	for index, field := range fields {
		value, err := canonicalCronField(field, ranges[index][0], ranges[index][1])
		if err != nil {
			return "", err
		}
		canonical[index] = value
	}
	if canonical[2] != "*" && canonical[4] != "*" {
		return "", ErrInvalidInput
	}
	return strings.Join(canonical, " "), nil
}

type cronTerm struct {
	start   int
	end     int
	step    int
	hasStep bool
}

func (term cronTerm) String(minimum, maximum int) string {
	base := strconv.Itoa(term.start)
	if term.start == minimum && term.end == maximum {
		base = "*"
	} else if term.start != term.end {
		base += "-" + strconv.Itoa(term.end)
	}
	if term.hasStep && term.step != 1 {
		base += "/" + strconv.Itoa(term.step)
	}
	return base
}

func canonicalCronField(raw string, minimum, maximum int) (string, error) {
	if raw == "" {
		return "", ErrInvalidInput
	}
	seen := make([]bool, maximum-minimum+1)
	terms := strings.Split(raw, ",")
	canonical := make([]string, 0, len(terms))
	for _, rawTerm := range terms {
		term, err := parseCronTerm(rawTerm, minimum, maximum)
		if err != nil {
			return "", err
		}
		for value := term.start; value <= term.end; value += term.step {
			position := value - minimum
			if seen[position] {
				return "", ErrInvalidInput
			}
			seen[position] = true
		}
		canonical = append(canonical, term.String(minimum, maximum))
	}
	return strings.Join(canonical, ","), nil
}

func parseCronTerm(raw string, minimum, maximum int) (cronTerm, error) {
	parts := strings.Split(raw, "/")
	if len(parts) > 2 || raw == "" || parts[0] == "" {
		return cronTerm{}, ErrInvalidInput
	}
	term, rangeOrStar, err := parseCronBase(parts[0], minimum, maximum)
	if err != nil {
		return cronTerm{}, err
	}
	if len(parts) == 1 {
		return term, nil
	}
	if !rangeOrStar || parts[1] == "" {
		return cronTerm{}, ErrInvalidInput
	}
	step, err := parseCronNumber(parts[1], 1, term.end-term.start+1)
	if err != nil {
		return cronTerm{}, err
	}
	term.step = step
	term.hasStep = true
	return term, nil
}

func parseCronBase(raw string, minimum, maximum int) (cronTerm, bool, error) {
	if raw == "*" {
		return cronTerm{start: minimum, end: maximum, step: 1}, true, nil
	}
	parts := strings.Split(raw, "-")
	if len(parts) == 1 {
		value, err := parseCronNumber(parts[0], minimum, maximum)
		return cronTerm{start: value, end: value, step: 1}, false, err
	}
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return cronTerm{}, false, ErrInvalidInput
	}
	start, err := parseCronNumber(parts[0], minimum, maximum)
	if err != nil {
		return cronTerm{}, false, err
	}
	end, err := parseCronNumber(parts[1], minimum, maximum)
	if err != nil || start > end {
		return cronTerm{}, false, ErrInvalidInput
	}
	return cronTerm{start: start, end: end, step: 1}, true, nil
}

func parseCronNumber(raw string, minimum, maximum int) (int, error) {
	if raw == "" {
		return 0, ErrInvalidInput
	}
	for _, character := range raw {
		if character < '0' || character > '9' {
			return 0, ErrInvalidInput
		}
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < minimum || value > maximum {
		return 0, ErrInvalidInput
	}
	return value, nil
}

func validateGroup(group Group) error {
	if group.ID <= 0 || group.Version <= 0 || group.CreatedBy <= 0 || group.CreatedAt.IsZero() || group.UpdatedAt.IsZero() || group.CreatedAt.After(group.UpdatedAt) {
		return ErrUnavailable
	}
	name, err := normalizeGroupName(group.Name)
	if err != nil || name != group.Name {
		return ErrUnavailable
	}
	if _, err = normalizeSortOrder(group.SortOrder); err != nil {
		return ErrUnavailable
	}
	return nil
}

func validateGroups(groups []Group) error {
	for index, group := range groups {
		if err := validateGroup(group); err != nil {
			return err
		}
		if index > 0 {
			previous := groups[index-1]
			if previous.SortOrder > group.SortOrder || (previous.SortOrder == group.SortOrder && previous.ID >= group.ID) {
				return ErrUnavailable
			}
		}
	}
	return nil
}

func validateMetadata(metadata PackageMetadata) error {
	if metadata.SegmentID <= 0 || metadata.Version <= 0 || metadata.CreatedBy < 0 || metadata.UpdatedBy < 0 ||
		metadata.CreatedAt.IsZero() || metadata.UpdatedAt.IsZero() || metadata.CreatedAt.After(metadata.UpdatedAt) {
		return ErrUnavailable
	}
	if metadata.GroupID != nil && *metadata.GroupID <= 0 {
		return ErrUnavailable
	}
	if metadata.Lifecycle != PackagePaused && metadata.Lifecycle != PackageActive && metadata.Lifecycle != PackageArchived {
		return ErrUnavailable
	}
	return nil
}

func packageFrom(metadata PackageMetadata, segment segmentport.Segment) (Package, error) {
	if err := validateMetadata(metadata); err != nil || int64(segment.ID) != metadata.SegmentID || segment.ID <= 0 || segment.MemberCount < 0 ||
		segment.CreatedAt.IsZero() || segment.UpdatedAt.IsZero() || segment.CreatedAt.After(segment.UpdatedAt) {
		return Package{}, ErrUnavailable
	}
	name, err := normalizePackageName(segment.Name)
	if err != nil || name != segment.Name {
		return Package{}, ErrUnavailable
	}
	definition, err := canonicalDefinition(segment.Definition)
	if err != nil {
		return Package{}, ErrUnavailable
	}
	cron, err := canonicalRefreshCron(segment.RefreshMode, segment.RefreshCron)
	if err != nil {
		return Package{}, ErrUnavailable
	}
	if segment.RefreshStatus != segmentport.RefreshStatusIdle && segment.RefreshStatus != segmentport.RefreshStatusRunning && segment.RefreshStatus != segmentport.RefreshStatusFailed {
		return Package{}, ErrUnavailable
	}
	switch metadata.Lifecycle {
	case PackageArchived:
		if segment.LifecycleStatus != segmentport.LifecycleStatusArchived {
			return Package{}, ErrUnavailable
		}
	case PackagePaused, PackageActive:
		if segment.LifecycleStatus != segmentport.LifecycleStatusActive {
			return Package{}, ErrUnavailable
		}
	default:
		return Package{}, ErrUnavailable
	}
	updatedAt := segment.UpdatedAt
	if metadata.UpdatedAt.After(updatedAt) {
		updatedAt = metadata.UpdatedAt
	}
	return Package{
		ID: metadata.SegmentID, Name: name, Definition: definition, GroupID: cloneInt64(metadata.GroupID),
		Lifecycle: metadata.Lifecycle, Version: metadata.Version, RefreshMode: segment.RefreshMode,
		RefreshCron: cloneString(cron), MemberCount: segment.MemberCount, RefreshedAt: cloneTime(segment.RefreshedAt),
		RefreshStatus: segment.RefreshStatus, CreatedAt: segment.CreatedAt, UpdatedAt: updatedAt,
	}, nil
}

func validateWriteModel(model PackageWriteModel) error {
	if err := validateMetadata(model.Metadata); err != nil || model.SegmentID != model.Metadata.SegmentID ||
		model.SegmentID <= 0 || model.Metadata.Lifecycle == PackageArchived && model.SegmentLifecycle != segmentport.LifecycleStatusArchived ||
		model.Metadata.Lifecycle != PackageArchived && model.SegmentLifecycle != segmentport.LifecycleStatusActive {
		return ErrUnavailable
	}
	name, err := normalizePackageName(model.Name)
	if err != nil || name != model.Name {
		return ErrUnavailable
	}
	if _, err = canonicalDefinition(model.Definition); err != nil {
		return ErrUnavailable
	}
	if _, err = canonicalRefreshCron(model.RefreshMode, model.RefreshCron); err != nil {
		return ErrUnavailable
	}
	return nil
}

func packageSummary(item Package) PackageSummary {
	return PackageSummary{
		ID: item.ID, Name: item.Name, GroupID: cloneInt64(item.GroupID), Lifecycle: item.Lifecycle,
		Version: item.Version, RefreshMode: item.RefreshMode, RefreshCron: cloneString(item.RefreshCron),
		MemberCount: item.MemberCount, RefreshedAt: cloneTime(item.RefreshedAt), RefreshStatus: item.RefreshStatus,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

func packageMutation(model PackageWriteModel, memberCount *int64) PackageMutation {
	return PackageMutation{
		ID: model.SegmentID, Name: model.Name, GroupID: cloneInt64(model.Metadata.GroupID),
		Lifecycle: model.Metadata.Lifecycle, Version: model.Metadata.Version, RefreshMode: model.RefreshMode,
		RefreshCron: cloneString(model.RefreshCron), MemberCount: cloneInt64(memberCount),
		CreatedAt: model.Metadata.CreatedAt, UpdatedAt: model.Metadata.UpdatedAt,
	}
}

func deterministicCopyName(source string, ordinal int) (string, error) {
	if ordinal < 1 {
		return "", ErrInvalidInput
	}
	suffix := " 副本"
	if ordinal > 1 {
		suffix = fmt.Sprintf(" 副本 (%d)", ordinal)
	}
	available := MaximumPackageNameRunes - utf8.RuneCountInString(suffix)
	if available < 1 {
		return "", ErrUnavailable
	}
	runes := []rune(strings.TrimSpace(source))
	if len(runes) > available {
		runes = runes[:available]
	}
	prefix := strings.TrimSpace(string(runes))
	if prefix == "" {
		prefix = "套餐"
	}
	return normalizePackageName(prefix + suffix)
}

func digestJSON(value any) ([32]byte, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return [32]byte{}, errors.Join(ErrUnavailable, err)
	}
	return sha256.Sum256(payload), nil
}

func cloneDefinition(value segmentport.Definition) segmentport.Definition {
	return append(segmentport.Definition(nil), value...)
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneWriteModel(value PackageWriteModel) PackageWriteModel {
	value.Definition = cloneDefinition(value.Definition)
	value.RefreshCron = cloneString(value.RefreshCron)
	value.Metadata.GroupID = cloneInt64(value.Metadata.GroupID)
	return value
}

func nilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
