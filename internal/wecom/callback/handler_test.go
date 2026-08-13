package callback

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

type officialFixture struct {
	CorpID            string `json:"corp_id"`
	Token             string `json:"token"`
	EncodingAESKey    string `json:"encoding_aes_key"`
	Timestamp         string `json:"timestamp"`
	Nonce             string `json:"nonce"`
	Signature         string `json:"msg_signature"`
	EchoStr           string `json:"echostr"`
	ExpectedPlaintext string `json:"expected_plaintext"`
}

func TestOfficialFormatURLVerificationFixture(t *testing.T) {
	fixture := loadFixture(t, "official_url_verification.json")
	handler := officialHandler(t, fixture)
	request := httptest.NewRequest(http.MethodGet, EventsPath+"?"+url.Values{
		"msg_signature": {fixture.Signature}, "timestamp": {fixture.Timestamp}, "nonce": {fixture.Nonce}, "echostr": {fixture.EchoStr},
	}.Encode(), nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != fixture.ExpectedPlaintext || response.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Fatalf("URL verification response = %d, %q, %q", response.Code, response.Body.String(), response.Header().Get("Content-Type"))
	}
}

func TestOfficialFormatMessageFixtureReturnsEncryptedSuccess(t *testing.T) {
	fixture := loadFixture(t, "official_message_callback.json")
	body, err := os.ReadFile(filepath.Join("testdata", "official_message_callback.xml"))
	if err != nil {
		t.Fatal(err)
	}
	handler, appender := officialHandlerWithAppender(t, fixture)
	if plaintext, err := handler.decrypt(encryptedFixtureBody(t, body, fixture.CorpID)); err != nil || string(plaintext) != fixture.ExpectedPlaintext {
		t.Fatalf("official fixture decrypt = %q, %v", plaintext, err)
	}
	request := httptest.NewRequest(http.MethodPost, ExternalContactCallbackPath+"?"+url.Values{
		"msg_signature": {fixture.Signature}, "timestamp": {fixture.Timestamp}, "nonce": {fixture.Nonce},
	}.Encode(), strings.NewReader(string(body)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/xml; charset=utf-8" {
		t.Fatalf("message callback response = %d, %q", response.Code, response.Body.String())
	}
	var reply struct {
		Encrypt   string `xml:"Encrypt"`
		Signature string `xml:"MsgSignature"`
		Timestamp string `xml:"TimeStamp"`
		Nonce     string `xml:"Nonce"`
	}
	if err := xml.Unmarshal(response.Body.Bytes(), &reply); err != nil || !handler.validSignature(reply.Signature, reply.Timestamp, reply.Nonce, reply.Encrypt) {
		t.Fatalf("encrypted success reply = %q, %v", response.Body.String(), err)
	}
	plaintext, err := handler.decrypt(reply.Encrypt)
	if err != nil || string(plaintext) != "success" {
		t.Fatalf("success reply plaintext = %q, %v", plaintext, err)
	}
	replay := httptest.NewRecorder()
	handler.ServeHTTP(replay, httptest.NewRequest(http.MethodPost, ExternalContactCallbackPath+"?"+url.Values{
		"msg_signature": {fixture.Signature}, "timestamp": {fixture.Timestamp}, "nonce": {fixture.Nonce},
	}.Encode(), strings.NewReader(string(body))))
	if replay.Code != http.StatusOK || len(appender.events) != 1 {
		t.Fatalf("replay response/facts = %d/%d, want 200/1", replay.Code, len(appender.events))
	}
}

func TestCallbackNegativeCasesAreStableAndNeverAuthenticate(t *testing.T) {
	fixture := loadFixture(t, "official_url_verification.json")
	handler := officialHandler(t, fixture)
	validQuery := url.Values{"msg_signature": {fixture.Signature}, "timestamp": {fixture.Timestamp}, "nonce": {fixture.Nonce}, "echostr": {fixture.EchoStr}}
	otherRecipient := otherRecipientCiphertext(t, fixture)
	tests := []struct {
		name    string
		method  string
		path    string
		body    io.Reader
		want    int
		message string
	}{
		{name: "bad signature", method: http.MethodGet, path: EventsPath + "?" + mutate(validQuery, "msg_signature", "0000000000000000000000000000000000000000").Encode(), want: http.StatusBadRequest, message: "invalid callback"},
		{name: "wrong encrypted recipient", method: http.MethodPost, path: ExternalContactCallbackPath + "?" + url.Values{"msg_signature": {fixture.Signature}, "timestamp": {fixture.Timestamp}, "nonce": {fixture.Nonce}}.Encode(), body: strings.NewReader("<xml><ToUserName><![CDATA[other-corp]]></ToUserName><Encrypt><![CDATA[" + fixture.EchoStr + "]]></Encrypt></xml>"), want: http.StatusBadRequest, message: "invalid callback"},
		{name: "wrong decrypted receiveid", method: http.MethodPost, path: ExternalContactCallbackPath + "?" + url.Values{"msg_signature": {signatureFor(fixture.Token, fixture.Timestamp, fixture.Nonce, otherRecipient)}, "timestamp": {fixture.Timestamp}, "nonce": {fixture.Nonce}}.Encode(), body: strings.NewReader("<xml><ToUserName><![CDATA[" + fixture.CorpID + "]]></ToUserName><Encrypt><![CDATA[" + otherRecipient + "]]></Encrypt></xml>"), want: http.StatusBadRequest, message: "invalid callback"},
		{name: "missing callback configuration", method: http.MethodGet, path: EventsPath, want: http.StatusServiceUnavailable, message: "callback unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target := handler
			if test.name == "missing callback configuration" {
				var err error
				target, err = NewHandler(Config{}, Options{})
				if err != nil {
					t.Fatal(err)
				}
			}
			response := httptest.NewRecorder()
			target.ServeHTTP(response, httptest.NewRequest(test.method, test.path, test.body))
			if response.Code != test.want || response.Body.String() != test.message || strings.Contains(response.Body.String(), fixture.Token) || strings.Contains(response.Body.String(), fixture.EncodingAESKey) {
				t.Fatalf("response = %d, %q", response.Code, response.Body.String())
			}
		})
	}
}

func TestCallbackRejectsUnsupportedEventWithoutEncryptedACK(t *testing.T) {
	fixture := loadFixture(t, "official_message_callback.json")
	appender := &deduplicatingCallbackAppender{}
	dispatcher, err := NewEventDispatcher(immediateCallbackUoW{}, appender)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler(Config{Enabled: true, CorpID: fixture.CorpID, Token: fixture.Token, EncodingAESKey: fixture.EncodingAESKey}, Options{Dispatcher: dispatcher})
	if err != nil {
		t.Fatal(err)
	}
	plaintext := []byte("<xml><ToUserName><![CDATA[" + fixture.CorpID + "]]></ToUserName><CreateTime>1700000001</CreateTime><MsgType><![CDATA[event]]></MsgType><Event><![CDATA[change_external_contact]]></Event></xml>")
	encrypted, err := handler.encrypt(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	body := "<xml><ToUserName><![CDATA[" + fixture.CorpID + "]]></ToUserName><Encrypt><![CDATA[" + encrypted + "]]></Encrypt></xml>"
	request := httptest.NewRequest(http.MethodPost, EventsPath+"?"+url.Values{
		"msg_signature": {signatureFor(fixture.Token, fixture.Timestamp, fixture.Nonce, encrypted)}, "timestamp": {fixture.Timestamp}, "nonce": {fixture.Nonce},
	}.Encode(), strings.NewReader(body))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || response.Body.String() != "invalid callback" || strings.Contains(response.Body.String(), "success") || len(appender.events) != 1 {
		t.Fatalf("unsupported event response = %d, %q", response.Code, response.Body.String())
	}
	for _, event := range appender.events {
		if event.Type != rejectedCallbackEventType {
			t.Fatalf("unknown event was not auditable: %#v", event)
		}
	}
}

func otherRecipientCiphertext(t *testing.T, fixture officialFixture) string {
	t.Helper()
	handler, err := NewHandler(Config{Enabled: true, CorpID: "other-corp", Token: fixture.Token, EncodingAESKey: fixture.EncodingAESKey}, Options{
		RandomByte: func(target []byte) error { return nil },
		Dispatcher: &EventDispatcher{uow: immediateCallbackUoW{}, events: &deduplicatingCallbackAppender{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := handler.encrypt([]byte("<xml><ToUserName><![CDATA[" + fixture.CorpID + "]]></ToUserName></xml>"))
	if err != nil {
		t.Fatal(err)
	}
	return ciphertext
}

func signatureFor(token, timestamp, nonce, encrypted string) string {
	parts := []string{token, timestamp, nonce, encrypted}
	sort.Strings(parts)
	digest := sha1.Sum([]byte(strings.Join(parts, "")))
	return hex.EncodeToString(digest[:])
}

func TestHandlerRejectsPartialConfiguration(t *testing.T) {
	if _, err := NewHandler(Config{Enabled: true, CorpID: "wx5823bf96d3bd56c7", Token: "token"}, Options{}); err == nil {
		t.Fatal("NewHandler accepted partial configuration")
	}
	if _, err := NewHandler(Config{Enabled: true, CorpID: "wx5823bf96d3bd56c7", Token: "token", EncodingAESKey: "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG"}, Options{}); err == nil {
		t.Fatal("NewHandler accepted enabled callback without a durable dispatcher")
	}
}

func loadFixture(t *testing.T, name string) officialFixture {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var fixture officialFixture
	if err := json.Unmarshal(contents, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func officialHandler(t *testing.T, fixture officialFixture) *Handler {
	t.Helper()
	handler, _ := officialHandlerWithAppender(t, fixture)
	return handler
}

func officialHandlerWithAppender(t *testing.T, fixture officialFixture) (*Handler, *deduplicatingCallbackAppender) {
	t.Helper()
	appender := &deduplicatingCallbackAppender{}
	dispatcher, err := NewEventDispatcher(immediateCallbackUoW{}, appender)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler(Config{Enabled: true, CorpID: fixture.CorpID, Token: fixture.Token, EncodingAESKey: fixture.EncodingAESKey}, Options{
		Clock: func() time.Time { return time.Unix(1700000001, 0) },
		RandomByte: func(target []byte) error {
			for index := range target {
				target[index] = byte(index)
			}
			return nil
		},
		Nonce:      func() string { return "reply-nonce" },
		Dispatcher: dispatcher,
	})
	if err != nil {
		t.Fatal(err)
	}
	return handler, appender
}

func encryptedFixtureBody(t *testing.T, body []byte, corpID string) string {
	t.Helper()
	encrypted, err := encryptedEnvelope(body, corpID)
	if err != nil {
		t.Fatal(err)
	}
	return encrypted
}

func mutate(values url.Values, key, value string) url.Values {
	copy := make(url.Values, len(values))
	for currentKey, currentValues := range values {
		copy[currentKey] = append([]string(nil), currentValues...)
	}
	copy.Set(key, value)
	return copy
}
