package membergrid

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"time"
)

const (
	cursorPrefix               = "mg2."
	cursorPayloadSize          = 46
	selectedCursorPrefix       = "mg3."
	selectedCursorSize         = 49
	minimumCursorKey           = 32
	cursorVersion         byte = 2
	selectedCursorVersion      = 3
)

var cursorKeyDomain = []byte("AI-CRM-v2/membergrid/cursor/v2\x00")

type CursorCodec struct {
	aead   cipher.AEAD
	random io.Reader
}

func NewCursorCodec(secret []byte) (*CursorCodec, error) {
	return newCursorCodec(secret, rand.Reader)
}

func newCursorCodec(secret []byte, random io.Reader) (*CursorCodec, error) {
	if len(secret) < minimumCursorKey || random == nil {
		return nil, errors.New("member grid cursor secret must contain at least 32 bytes")
	}
	material := make([]byte, 0, len(cursorKeyDomain)+len(secret))
	material = append(material, cursorKeyDomain...)
	material = append(material, secret...)
	key := sha256.Sum256(material)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &CursorCodec{aead: aead, random: random}, nil
}

func (codec *CursorCodec) Encode(productID int64, state StateFilter, source SourceFilter, limit int, position Position) (string, error) {
	if codec == nil || codec.aead == nil || codec.random == nil || productID < 1 ||
		!state.validCanonicalGridState() || !source.valid() || limit < 1 || limit > MaximumLimit || !validMemberRef(position.MemberRef) || position.UpdatedAt.IsZero() {
		return "", ErrInvalidCursor
	}
	payload := make([]byte, cursorPayloadSize)
	payload[0] = cursorVersion
	payload[1] = encodeCursorState(state)
	payload[2] = encodeCursorSource(source)
	payload[3] = byte(limit)
	binary.BigEndian.PutUint64(payload[4:12], uint64(productID))
	binary.BigEndian.PutUint64(payload[12:20], uint64(position.UpdatedAt.UTC().UnixMicro()))
	copy(payload[20:46], position.MemberRef)

	nonce := make([]byte, codec.aead.NonceSize())
	if _, err := io.ReadFull(codec.random, nonce); err != nil {
		return "", errors.Join(ErrUnavailable, err)
	}
	sealed := codec.aead.Seal(nil, nonce, payload, []byte(cursorPrefix))
	token := append(nonce, sealed...)
	return cursorPrefix + base64.RawURLEncoding.EncodeToString(token), nil
}

func (codec *CursorCodec) Decode(token string, productID int64, state StateFilter, source SourceFilter, limit int) (Position, error) {
	if codec == nil || codec.aead == nil || productID < 1 || !state.validCanonicalGridState() || !source.valid() || limit < 1 || limit > MaximumLimit ||
		len(token) <= len(cursorPrefix) || len(token) > 256 || token[:len(cursorPrefix)] != cursorPrefix {
		return Position{}, ErrInvalidCursor
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(token[len(cursorPrefix):])
	if err != nil || len(raw) != codec.aead.NonceSize()+cursorPayloadSize+codec.aead.Overhead() {
		return Position{}, ErrInvalidCursor
	}
	nonce := raw[:codec.aead.NonceSize()]
	payload, err := codec.aead.Open(nil, nonce, raw[codec.aead.NonceSize():], []byte(cursorPrefix))
	if err != nil || len(payload) != cursorPayloadSize || payload[0] != cursorVersion {
		return Position{}, ErrInvalidCursor
	}
	decodedState, ok := decodeCursorState(payload[1])
	decodedSource, sourceOK := decodeCursorSource(payload[2])
	decodedLimit := int(payload[3])
	decodedProductID := int64(binary.BigEndian.Uint64(payload[4:12]))
	updatedAtMicros := int64(binary.BigEndian.Uint64(payload[12:20]))
	memberRef := string(payload[20:46])
	if !ok || !sourceOK || decodedState != state || decodedSource != source || decodedLimit != limit || decodedProductID != productID || decodedProductID < 1 || !validMemberRef(memberRef) {
		return Position{}, ErrInvalidCursor
	}
	updatedAt := time.UnixMicro(updatedAtMicros).UTC()
	if updatedAt.IsZero() {
		return Position{}, ErrInvalidCursor
	}
	return Position{UpdatedAt: updatedAt, MemberRef: memberRef}, nil
}

func (codec *CursorCodec) encodeSelected(productID int64, state StateFilter, source SourceFilter, limit int, selection querySelection, position selectedPosition) (string, error) {
	if codec == nil || codec.aead == nil || codec.random == nil || productID < 1 || !state.validCanonicalGridState() || !source.valid() ||
		limit < 1 || limit > MaximumLimit || !selection.Sort.valid() || !selection.GroupBy.valid() || !validMemberRef(position.MemberRef) || position.SortAt.IsZero() ||
		(selection.GroupBy == queryGroupState && stateGroupRank(position.GroupState) == 0) || (selection.GroupBy == queryGroupNone && position.GroupState != "") {
		return "", ErrInvalidCursor
	}
	payload := make([]byte, selectedCursorSize)
	payload[0] = selectedCursorVersion
	payload[1] = encodeCursorState(state)
	payload[2] = encodeCursorSource(source)
	payload[3] = encodeSelectedSort(selection.Sort)
	payload[4] = encodeSelectedGroup(selection.GroupBy)
	payload[5] = byte(limit)
	binary.BigEndian.PutUint64(payload[6:14], uint64(productID))
	binary.BigEndian.PutUint64(payload[14:22], uint64(position.SortAt.UTC().UnixMicro()))
	payload[22] = encodeCursorState(position.GroupState)
	copy(payload[23:49], position.MemberRef)

	nonce := make([]byte, codec.aead.NonceSize())
	if _, err := io.ReadFull(codec.random, nonce); err != nil {
		return "", errors.Join(ErrUnavailable, err)
	}
	sealed := codec.aead.Seal(nil, nonce, payload, []byte(selectedCursorPrefix))
	token := append(nonce, sealed...)
	return selectedCursorPrefix + base64.RawURLEncoding.EncodeToString(token), nil
}

func (codec *CursorCodec) decodeSelected(token string, productID int64, state StateFilter, source SourceFilter, limit int, selection querySelection) (selectedPosition, error) {
	if codec == nil || codec.aead == nil || productID < 1 || !state.validCanonicalGridState() || !source.valid() || limit < 1 || limit > MaximumLimit ||
		!selection.Sort.valid() || !selection.GroupBy.valid() || len(token) <= len(selectedCursorPrefix) || len(token) > 256 || token[:len(selectedCursorPrefix)] != selectedCursorPrefix {
		return selectedPosition{}, ErrInvalidCursor
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(token[len(selectedCursorPrefix):])
	if err != nil || len(raw) != codec.aead.NonceSize()+selectedCursorSize+codec.aead.Overhead() {
		return selectedPosition{}, ErrInvalidCursor
	}
	nonce := raw[:codec.aead.NonceSize()]
	payload, err := codec.aead.Open(nil, nonce, raw[codec.aead.NonceSize():], []byte(selectedCursorPrefix))
	if err != nil || len(payload) != selectedCursorSize || payload[0] != selectedCursorVersion {
		return selectedPosition{}, ErrInvalidCursor
	}
	decodedState, stateOK := decodeCursorState(payload[1])
	decodedSource, sourceOK := decodeCursorSource(payload[2])
	decodedSort, sortOK := decodeSelectedSort(payload[3])
	decodedGroup, groupOK := decodeSelectedGroup(payload[4])
	decodedLimit := int(payload[5])
	decodedProductID := int64(binary.BigEndian.Uint64(payload[6:14]))
	sortAt := time.UnixMicro(int64(binary.BigEndian.Uint64(payload[14:22]))).UTC()
	groupState, groupStateOK := decodeCursorState(payload[22])
	memberRef := string(payload[23:49])
	if !stateOK || !sourceOK || !sortOK || !groupOK || decodedState != state || decodedSource != source || decodedSort != selection.Sort ||
		decodedGroup != selection.GroupBy || decodedLimit != limit || decodedProductID != productID || decodedProductID < 1 || sortAt.IsZero() || !validMemberRef(memberRef) ||
		(selection.GroupBy == queryGroupState && (!groupStateOK || stateGroupRank(groupState) == 0)) ||
		(selection.GroupBy == queryGroupNone && (payload[22] != 0 || groupState != "")) {
		return selectedPosition{}, ErrInvalidCursor
	}
	return selectedPosition{SortAt: sortAt, MemberRef: memberRef, GroupState: groupState}, nil
}

func encodeCursorState(state StateFilter) byte {
	switch state {
	case StateActive:
		return 1
	case StateExpired:
		return 2
	case StateRemoved:
		return 3
	case StateAll:
		return 4
	default:
		return 0
	}
}

func decodeCursorState(value byte) (StateFilter, bool) {
	switch value {
	case 1:
		return StateActive, true
	case 2:
		return StateExpired, true
	case 3:
		return StateRemoved, true
	case 4:
		return StateAll, true
	default:
		return "", false
	}
}

func encodeCursorSource(source SourceFilter) byte {
	switch source {
	case SourceAny:
		return 0
	case SourceManual:
		return 1
	case SourcePaidOrder:
		return 2
	default:
		return 255
	}
}

func decodeCursorSource(value byte) (SourceFilter, bool) {
	switch value {
	case 0:
		return SourceAny, true
	case 1:
		return SourceManual, true
	case 2:
		return SourcePaidOrder, true
	default:
		return "", false
	}
}

func encodeSelectedSort(sort querySort) byte {
	switch sort {
	case querySortUpdatedAtDesc:
		return 1
	case querySortStartsAtDesc:
		return 2
	default:
		return 0
	}
}

func decodeSelectedSort(value byte) (querySort, bool) {
	switch value {
	case 1:
		return querySortUpdatedAtDesc, true
	case 2:
		return querySortStartsAtDesc, true
	default:
		return "", false
	}
}

func encodeSelectedGroup(groupBy queryGroupBy) byte {
	switch groupBy {
	case queryGroupNone:
		return 0
	case queryGroupState:
		return 1
	default:
		return 255
	}
}

func decodeSelectedGroup(value byte) (queryGroupBy, bool) {
	switch value {
	case 0:
		return queryGroupNone, true
	case 1:
		return queryGroupState, true
	default:
		return "", false
	}
}
