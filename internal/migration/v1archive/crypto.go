package v1archive

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

func equalBytes(left, right []byte) bool {
	return len(left) == len(right) && hmac.Equal(left, right)
}

func hmacDigest(key []byte, domain string, values ...[]byte) ([sha256.Size]byte, error) {
	if len(key) < sha256.Size || domain == "" {
		return [sha256.Size]byte{}, ErrInvalidConfig
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(domain))
	for _, value := range values {
		var length [4]byte
		binary.BigEndian.PutUint32(length[:], uint32(len(value)))
		_, _ = mac.Write(length[:])
		_, _ = mac.Write(value)
	}
	var result [sha256.Size]byte
	copy(result[:], mac.Sum(nil))
	return result, nil
}

func SourceKeyHMAC(key []byte, table string, sourceKeyJSON []byte) ([sha256.Size]byte, error) {
	if table == "" || len(sourceKeyJSON) == 0 || !json.Valid(sourceKeyJSON) {
		return [sha256.Size]byte{}, ErrInvalidConfig
	}
	return hmacDigest(key, "aicrm/v1archive/source-key/v1", []byte(table), sourceKeyJSON)
}

func PayloadHMAC(key []byte, table string, payloadJSON []byte) ([sha256.Size]byte, error) {
	if table == "" || len(payloadJSON) == 0 || !json.Valid(payloadJSON) {
		return [sha256.Size]byte{}, ErrInvalidConfig
	}
	return hmacDigest(key, "aicrm/v1archive/payload/v1", []byte(table), payloadJSON)
}

func FieldHMAC(key []byte, table string, redactedPaths []string) ([sha256.Size]byte, error) {
	if table == "" {
		return [sha256.Size]byte{}, ErrInvalidConfig
	}
	encoded, err := json.Marshal(sortedStrings(redactedPaths))
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return hmacDigest(key, "aicrm/v1archive/fields/v1", []byte(table), encoded)
}

func ArchivePolicyDigest() [sha256.Size]byte {
	return sha256.Sum256([]byte("aicrm/v1archive/archive-only/no-activation/v1"))
}

func archiveAAD(runID, table string, sourceKey, payload, schema [sha256.Size]byte, version int) ([]byte, error) {
	if runID == "" || table == "" || version < 1 {
		return nil, ErrInvalidConfig
	}
	if len(runID) > 65535 || len(table) > 65535 {
		return nil, ErrInvalidConfig
	}
	result := make([]byte, 4+2+len(runID)+2+len(table)+sha256.Size*3)
	binary.BigEndian.PutUint32(result[:4], uint32(version))
	offset := 4
	binary.BigEndian.PutUint16(result[offset:offset+2], uint16(len(runID)))
	offset += 2
	offset += copy(result[offset:], runID)
	binary.BigEndian.PutUint16(result[offset:offset+2], uint16(len(table)))
	offset += 2
	offset += copy(result[offset:], table)
	offset += copy(result[offset:], sourceKey[:])
	offset += copy(result[offset:], payload[:])
	copy(result[offset:], schema[:])
	return result, nil
}

func encrypt(key, aad, plaintext []byte) (nonce, ciphertext []byte, err error) {
	if len(key) != 32 || len(plaintext) == 0 {
		return nil, nil, ErrInvalidConfig
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	return nonce, gcm.Seal(nil, nonce, plaintext, aad), nil
}

func DecryptRecord(key []byte, record Record) ([]byte, error) {
	if len(key) != 32 || record.RunID == "" || record.Table == "" || len(record.Nonce) != 12 || len(record.Ciphertext) <= 16 {
		return nil, ErrInvalidConfig
	}
	aad, err := archiveAAD(record.RunID, record.Table, record.SourceKeyHMAC, record.PayloadHMAC, record.SchemaDigest, record.ArchiveKeyVersion)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, record.Nonce, record.Ciphertext, aad)
}

func ArchiveRecord(config Config, run Run, table Table, ordinal int64, sourceKeyJSON, payloadJSON []byte) (Record, error) {
	if err := config.Validate(); err != nil {
		return Record{}, err
	}
	if err := run.Validate(); err != nil {
		return Record{}, err
	}
	if err := table.Validate(); err != nil || ordinal < 1 {
		return Record{}, ErrInvalidConfig
	}
	canonical, paths, err := RedactPayload(payloadJSON)
	if err != nil {
		return Record{}, err
	}
	sourceKey, err := SourceKeyHMAC(config.SourceHMACKey, table.Name, sourceKeyJSON)
	if err != nil {
		return Record{}, err
	}
	payload, err := PayloadHMAC(config.SourceHMACKey, table.Name, canonical)
	if err != nil {
		return Record{}, err
	}
	fields, err := FieldHMAC(config.SourceHMACKey, table.Name, paths)
	if err != nil {
		return Record{}, err
	}
	schema, err := tableDigest(table)
	if err != nil {
		return Record{}, err
	}
	aad, err := archiveAAD(run.ID, table.Name, sourceKey, payload, schema, config.ArchiveKeyVersion)
	if err != nil {
		return Record{}, err
	}
	nonce, ciphertext, err := encrypt(config.ArchiveKey, aad, canonical)
	if err != nil {
		return Record{}, err
	}
	return Record{RunID: run.ID, Table: table.Name, Ordinal: ordinal, SourceKeyHMAC: sourceKey, PayloadHMAC: payload, FieldHMAC: fields, SchemaDigest: schema, ArchiveKeyVersion: config.ArchiveKeyVersion, RedactedPaths: paths, Nonce: nonce, Ciphertext: ciphertext}, nil
}

func tableDigest(table Table) ([sha256.Size]byte, error) {
	if err := table.Validate(); err != nil {
		return [sha256.Size]byte{}, err
	}
	encoded, err := json.Marshal(table)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return sha256.Sum256(encoded), nil
}

func RedactPayload(payload []byte) ([]byte, []string, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, nil, fmt.Errorf("decode V1 archive payload: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, nil, errors.New("invalid V1 archive payload")
	}
	paths := make([]string, 0)
	redacted, err := redactValue(value, "", &paths)
	if err != nil {
		return nil, nil, err
	}
	encoded, err := json.Marshal(redacted)
	if err != nil {
		return nil, nil, fmt.Errorf("encode V1 archive payload: %w", err)
	}
	return encoded, sortedStrings(paths), nil
}

func redactValue(value any, path string, paths *[]string) (any, error) {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			if sensitiveField(key) {
				result[key] = "[REDACTED]"
				*paths = append(*paths, childPath)
				continue
			}
			value, err := redactValue(child, childPath, paths)
			if err != nil {
				return nil, err
			}
			result[key] = value
		}
		return result, nil
	case []any:
		result := make([]any, len(typed))
		for index, child := range typed {
			value, err := redactValue(child, fmt.Sprintf("%s[%d]", path, index), paths)
			if err != nil {
				return nil, err
			}
			result[index] = value
		}
		return result, nil
	default:
		return value, nil
	}
}

func sensitiveField(name string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(name, "-", "_"), " ", "_"))
	for _, marker := range []string{"password", "passwd", "secret", "token", "credential", "authorization", "cookie", "private_key", "access_key", "api_key", "appkey", "app_key", "encrypt_key", "encryption_key", "aes_key", "signing_key"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}
