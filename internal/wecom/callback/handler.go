// Package callback implements the provider-facing, cryptographic WeCom
// callback boundary. It verifies and decrypts provider messages, then hands
// only the plaintext to the bounded durable event dispatcher. It never
// synchronizes, attributes, or calls the provider.
package callback

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	EventsPath                  = "/api/wecom/events"
	ExternalContactCallbackPath = "/wecom/external-contact/callback"
	maxCallbackBodyBytes        = 1 << 20
	weComPKCS7BlockSize         = 32
)

var ErrInvalidConfig = errors.New("invalid WeCom callback configuration")

type Config struct {
	Enabled        bool
	CorpID         string
	Token          string
	EncodingAESKey string
}

type Options struct {
	Clock      func() time.Time
	RandomByte func([]byte) error
	Nonce      func() string
	Dispatcher MessageDispatcher
}

type Handler struct {
	enabled    bool
	corpID     string
	token      string
	key        []byte
	clock      func() time.Time
	random     func([]byte) error
	nonce      func() string
	dispatcher MessageDispatcher
}

func NewHandler(config Config, options Options) (*Handler, error) {
	if !config.Enabled {
		if config.CorpID != "" || config.Token != "" || config.EncodingAESKey != "" {
			return nil, ErrInvalidConfig
		}
		return &Handler{clock: defaultClock(options.Clock), random: defaultRandom(options.RandomByte), nonce: defaultNonce(options.Nonce)}, nil
	}
	if !validCorpID(config.CorpID) || !validToken(config.Token) || nilLike(options.Dispatcher) {
		return nil, ErrInvalidConfig
	}
	key, err := decodeEncodingAESKey(config.EncodingAESKey)
	if err != nil {
		return nil, ErrInvalidConfig
	}
	return &Handler{enabled: true, corpID: config.CorpID, token: config.Token, key: key, clock: defaultClock(options.Clock), random: defaultRandom(options.RandomByte), nonce: defaultNonce(options.Nonce), dispatcher: options.Dispatcher}, nil
}

func (handler *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || !handler.enabled {
		writeBoundaryError(writer, http.StatusServiceUnavailable, "callback unavailable")
		return
	}
	switch request.Method {
	case http.MethodGet:
		handler.verifyURL(writer, request)
	case http.MethodPost:
		handler.receiveMessage(writer, request)
	default:
		writeBoundaryError(writer, http.StatusBadRequest, "invalid callback")
	}
}

func (handler *Handler) verifyURL(writer http.ResponseWriter, request *http.Request) {
	signature, timestamp, nonce, encrypted, ok := callbackQuery(request)
	if !ok || !handler.validSignature(signature, timestamp, nonce, encrypted) {
		writeBoundaryError(writer, http.StatusBadRequest, "invalid callback")
		return
	}
	message, err := handler.decrypt(encrypted)
	if err != nil || !utf8.Valid(message) {
		writeBoundaryError(writer, http.StatusBadRequest, "invalid callback")
		return
	}
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(message)
}

func (handler *Handler) receiveMessage(writer http.ResponseWriter, request *http.Request) {
	signature, timestamp, nonce, _, ok := callbackQuery(request)
	if !ok {
		writeBoundaryError(writer, http.StatusBadRequest, "invalid callback")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, maxCallbackBodyBytes))
	if err != nil {
		writeBoundaryError(writer, http.StatusBadRequest, "invalid callback")
		return
	}
	encrypted, err := encryptedEnvelope(body, handler.corpID)
	if err != nil || !handler.validSignature(signature, timestamp, nonce, encrypted) {
		writeBoundaryError(writer, http.StatusBadRequest, "invalid callback")
		return
	}
	message, err := handler.decrypt(encrypted)
	if err != nil || !validMessageRecipient(message, handler.corpID) {
		writeBoundaryError(writer, http.StatusBadRequest, "invalid callback")
		return
	}
	if err = handler.dispatcher.Dispatch(request.Context(), message); err != nil {
		if errors.Is(err, ErrUnknownCallbackEvent) {
			writeBoundaryError(writer, http.StatusBadRequest, "invalid callback")
			return
		}
		writeBoundaryError(writer, http.StatusServiceUnavailable, "callback unavailable")
		return
	}
	reply, err := handler.encryptedSuccessReply()
	if err != nil {
		writeBoundaryError(writer, http.StatusServiceUnavailable, "callback unavailable")
		return
	}
	writer.Header().Set("Content-Type", "application/xml; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(reply)
}

func callbackQuery(request *http.Request) (signature, timestamp, nonce, encrypted string, ok bool) {
	if request == nil || request.URL == nil {
		return "", "", "", "", false
	}
	query := request.URL.Query()
	signature, signatureOK := oneQuery(query, "msg_signature", 40)
	timestamp, timestampOK := oneQuery(query, "timestamp", 128)
	nonce, nonceOK := oneQuery(query, "nonce", 128)
	encrypted, encryptedOK := oneQuery(query, "echostr", maxCallbackBodyBytes)
	if !signatureOK || !timestampOK || !nonceOK {
		return "", "", "", "", false
	}
	return signature, timestamp, nonce, encrypted, encryptedOK || request.Method == http.MethodPost
}

func oneQuery(query map[string][]string, key string, maximum int) (string, bool) {
	values, ok := query[key]
	if !ok || len(values) != 1 || values[0] == "" || len(values[0]) > maximum {
		return "", false
	}
	return values[0], true
}

func (handler *Handler) validSignature(signature, timestamp, nonce, encrypted string) bool {
	if len(signature) != sha1.Size*2 || len(timestamp) == 0 || len(nonce) == 0 || len(encrypted) == 0 {
		return false
	}
	if _, err := hex.DecodeString(signature); err != nil {
		return false
	}
	parts := []string{handler.token, timestamp, nonce, encrypted}
	sort.Strings(parts)
	digest := sha1.Sum([]byte(strings.Join(parts, "")))
	expected := hex.EncodeToString(digest[:])
	return subtle.ConstantTimeCompare([]byte(signature), []byte(expected)) == 1
}

func encryptedEnvelope(body []byte, corpID string) (string, error) {
	var envelope struct {
		XMLName    xml.Name `xml:"xml"`
		ToUserName string   `xml:"ToUserName"`
		Encrypt    string   `xml:"Encrypt"`
	}
	decoder := xml.NewDecoder(strings.NewReader(string(body)))
	if err := decoder.Decode(&envelope); err != nil || envelope.XMLName.Local != "xml" || envelope.ToUserName != corpID || envelope.Encrypt == "" {
		return "", errors.New("invalid encrypted envelope")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return "", errors.New("invalid encrypted envelope")
	}
	return envelope.Encrypt, nil
}

func validMessageRecipient(message []byte, corpID string) bool {
	var envelope struct {
		XMLName    xml.Name `xml:"xml"`
		ToUserName string   `xml:"ToUserName"`
	}
	decoder := xml.NewDecoder(strings.NewReader(string(message)))
	if err := decoder.Decode(&envelope); err != nil || envelope.XMLName.Local != "xml" || envelope.ToUserName != corpID {
		return false
	}
	return decoder.Decode(&struct{}{}) == io.EOF
}

func (handler *Handler) decrypt(encoded string) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return nil, errors.New("invalid ciphertext")
	}
	block, err := aes.NewCipher(handler.key)
	if err != nil {
		return nil, err
	}
	plaintext := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, handler.key[:aes.BlockSize]).CryptBlocks(plaintext, ciphertext)
	plaintext, err = unpadPKCS7(plaintext)
	if err != nil {
		return nil, err
	}
	if len(plaintext) < 20 {
		return nil, errors.New("invalid plaintext")
	}
	messageLength := int(binary.BigEndian.Uint32(plaintext[16:20]))
	messageEnd := 20 + messageLength
	if messageLength < 0 || messageEnd < 20 || messageEnd > len(plaintext) || messageEnd == len(plaintext) || string(plaintext[messageEnd:]) != handler.corpID {
		return nil, errors.New("invalid recipient")
	}
	return append([]byte(nil), plaintext[20:messageEnd]...), nil
}

func (handler *Handler) encryptedSuccessReply() ([]byte, error) {
	encoded, err := handler.encrypt([]byte("success"))
	if err != nil {
		return nil, err
	}
	timestamp := strconv.FormatInt(handler.clock().Unix(), 10)
	nonce := handler.nonce()
	if nonce == "" || len(nonce) > 128 {
		return nil, errors.New("invalid reply nonce")
	}
	parts := []string{handler.token, timestamp, nonce, encoded}
	sort.Strings(parts)
	digest := sha1.Sum([]byte(strings.Join(parts, "")))
	signature := hex.EncodeToString(digest[:])
	return []byte(fmt.Sprintf("<xml><Encrypt><![CDATA[%s]]></Encrypt><MsgSignature><![CDATA[%s]]></MsgSignature><TimeStamp>%s</TimeStamp><Nonce><![CDATA[%s]]></Nonce></xml>", encoded, signature, timestamp, nonce)), nil
}

func (handler *Handler) encrypt(message []byte) (string, error) {
	randomPrefix := make([]byte, 16)
	if err := handler.random(randomPrefix); err != nil {
		return "", err
	}
	payload := make([]byte, 20+len(message)+len(handler.corpID))
	copy(payload[:16], randomPrefix)
	binary.BigEndian.PutUint32(payload[16:20], uint32(len(message)))
	copy(payload[20:], message)
	copy(payload[20+len(message):], handler.corpID)
	padding := weComPKCS7BlockSize - len(payload)%weComPKCS7BlockSize
	payload = append(payload, bytesOf(padding, byte(padding))...)
	block, err := aes.NewCipher(handler.key)
	if err != nil {
		return "", err
	}
	ciphertext := make([]byte, len(payload))
	cipher.NewCBCEncrypter(block, handler.key[:aes.BlockSize]).CryptBlocks(ciphertext, payload)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func unpadPKCS7(value []byte) ([]byte, error) {
	if len(value) == 0 || len(value)%aes.BlockSize != 0 {
		return nil, errors.New("invalid padding")
	}
	padding := int(value[len(value)-1])
	if padding < 1 || padding > weComPKCS7BlockSize || padding > len(value) {
		return nil, errors.New("invalid padding")
	}
	for _, character := range value[len(value)-padding:] {
		if int(character) != padding {
			return nil, errors.New("invalid padding")
		}
	}
	return value[:len(value)-padding], nil
}

func bytesOf(length int, value byte) []byte {
	result := make([]byte, length)
	for index := range result {
		result[index] = value
	}
	return result
}

func decodeEncodingAESKey(value string) ([]byte, error) {
	if len(value) != 43 {
		return nil, errors.New("invalid AES key")
	}
	decoded, err := base64.StdEncoding.DecodeString(value + "=")
	if err != nil || len(decoded) != 32 {
		return nil, errors.New("invalid AES key")
	}
	return decoded, nil
}

func validCorpID(value string) bool {
	if len(value) == 0 || len(value) > 128 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '_' || character == '-') {
			return false
		}
	}
	return true
}

func validToken(value string) bool {
	return len(value) > 0 && len(value) <= 256 && strings.TrimSpace(value) == value
}

func defaultClock(clock func() time.Time) func() time.Time {
	if clock != nil {
		return clock
	}
	return time.Now
}

func defaultRandom(random func([]byte) error) func([]byte) error {
	if random != nil {
		return random
	}
	return func(target []byte) error { _, err := rand.Read(target); return err }
}

func defaultNonce(nonce func() string) func() string {
	if nonce != nil {
		return nonce
	}
	return func() string {
		var value [16]byte
		if _, err := rand.Read(value[:]); err != nil {
			return ""
		}
		return hex.EncodeToString(value[:])
	}
}

func writeBoundaryError(writer http.ResponseWriter, status int, message string) {
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writer.WriteHeader(status)
	_, _ = io.WriteString(writer, message)
}
