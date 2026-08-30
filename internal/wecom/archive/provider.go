package archive

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const helperFrame = "AICRM_SDK_RESULT="

type SDKConfig struct {
	CorpID, ArchiveSecret, PrivateKeyPath, SDKLibraryPath string
	PythonPath, HelperPath                                string
	Timeout                                               time.Duration
}

type SDKProvider struct {
	config     SDKConfig
	privateKey *rsa.PrivateKey
}

func NewSDKProvider(config SDKConfig) (*SDKProvider, error) {
	if !validID(config.CorpID) || config.ArchiveSecret == "" || strings.TrimSpace(config.ArchiveSecret) != config.ArchiveSecret ||
		config.PrivateKeyPath == "" || config.SDKLibraryPath == "" || config.PythonPath == "" || config.HelperPath == "" || config.Timeout < time.Second {
		return nil, ErrInvalidConfiguration
	}
	keyBytes, err := os.ReadFile(config.PrivateKeyPath)
	if err != nil {
		return nil, errors.Join(ErrInvalidConfiguration, err)
	}
	block, _ := pem.Decode(keyBytes)
	if block == nil {
		return nil, ErrInvalidConfiguration
	}
	key, err := parsePrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.Join(ErrInvalidConfiguration, err)
	}
	for _, path := range []string{config.SDKLibraryPath, config.PythonPath, config.HelperPath} {
		if info, statErr := os.Stat(path); statErr != nil || info.IsDir() {
			return nil, errors.Join(ErrInvalidConfiguration, statErr)
		}
	}
	return &SDKProvider{config: config, privateKey: key}, nil
}

func (provider *SDKProvider) FetchPage(ctx context.Context, seq int64, limit int) ([]EncryptedRecord, error) {
	if provider == nil || ctx == nil || seq < 0 || limit < 1 || limit > 1_000 {
		return nil, ErrSyncUnavailable
	}
	result, err := provider.runHelper(ctx, "fetch", map[string]any{
		"lib_path": provider.config.SDKLibraryPath, "corp_id": provider.config.CorpID,
		"archive_secret": provider.config.ArchiveSecret, "seq": seq, "limit": limit,
		"timeout": int(provider.config.Timeout.Seconds()),
	})
	if err != nil {
		return nil, err
	}
	payload, ok := result["payload"].(map[string]any)
	if !ok {
		return nil, ErrSyncUnavailable
	}
	encoded, err := json.Marshal(payload["chatdata"])
	if err != nil {
		return nil, ErrSyncUnavailable
	}
	var records []EncryptedRecord
	if err = json.Unmarshal(encoded, &records); err != nil {
		return nil, ErrSyncUnavailable
	}
	for index, record := range records {
		if record.Seq < 1 || record.PublicKeyVersion < 1 || record.EncryptedKey == "" || record.EncryptedMessage == "" || index > 0 && record.Seq <= records[index-1].Seq {
			return nil, ErrSyncUnavailable
		}
	}
	return records, nil
}

func (provider *SDKProvider) Decrypt(ctx context.Context, records []EncryptedRecord) ([]map[string]any, error) {
	if provider == nil || ctx == nil || len(records) > 1_000 {
		return nil, ErrSyncUnavailable
	}
	items := make([]map[string]string, len(records))
	for index, record := range records {
		randomKey, err := provider.decryptRandomKey(record.EncryptedKey)
		if err != nil {
			return nil, err
		}
		items[index] = map[string]string{"random_key": randomKey, "encrypt_chat_msg": record.EncryptedMessage}
	}
	result, err := provider.runHelper(ctx, "decrypt", map[string]any{"lib_path": provider.config.SDKLibraryPath, "items": items})
	if err != nil {
		return nil, err
	}
	payloads, ok := result["payloads"].([]any)
	if !ok || len(payloads) != len(records) {
		return nil, ErrSyncUnavailable
	}
	decrypted := make([]map[string]any, len(payloads))
	for index, payload := range payloads {
		value, ok := payload.(map[string]any)
		if !ok {
			return nil, ErrSyncUnavailable
		}
		decrypted[index] = value
	}
	return decrypted, nil
}

func (provider *SDKProvider) decryptRandomKey(encoded string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", ErrSyncUnavailable
	}
	decryptions := []func() ([]byte, error){
		func() ([]byte, error) { return rsa.DecryptPKCS1v15(rand.Reader, provider.privateKey, ciphertext) },
		func() ([]byte, error) {
			return rsa.DecryptOAEP(sha1.New(), rand.Reader, provider.privateKey, ciphertext, nil)
		},
		func() ([]byte, error) {
			return rsa.DecryptOAEP(sha256.New(), rand.Reader, provider.privateKey, ciphertext, nil)
		},
	}
	for _, decrypt := range decryptions {
		plain, decryptErr := decrypt()
		if decryptErr == nil && len(plain) > 0 {
			return string(plain), nil
		}
	}
	return "", ErrSyncUnavailable
}

func (provider *SDKProvider) runHelper(ctx context.Context, operation string, payload map[string]any) (map[string]any, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, ErrSyncUnavailable
	}
	callCtx, cancel := context.WithTimeout(ctx, provider.config.Timeout+15*time.Second)
	defer cancel()
	command := exec.CommandContext(callCtx, provider.config.PythonPath, provider.config.HelperPath, operation)
	command.Stdin = bytes.NewReader(encoded)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &bytes.Buffer{}
	if err = command.Run(); err != nil {
		return nil, errors.Join(ErrSyncUnavailable, err)
	}
	var frame string
	for _, line := range strings.Split(output.String(), "\n") {
		if strings.HasPrefix(line, helperFrame) {
			frame = strings.TrimPrefix(line, helperFrame)
		}
	}
	if frame == "" {
		return nil, ErrSyncUnavailable
	}
	var result map[string]any
	if err = json.Unmarshal([]byte(frame), &result); err != nil || result["ok"] != true {
		return nil, fmt.Errorf("%w: sdk helper failed", ErrSyncUnavailable)
	}
	return result, nil
}

func parsePrivateKey(der []byte) (*rsa.PrivateKey, error) {
	if key, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, ErrInvalidConfiguration
	}
	return key, nil
}
