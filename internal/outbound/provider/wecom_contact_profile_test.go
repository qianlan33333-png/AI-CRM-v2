package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
)

type profileTokenFake struct {
	token, refresh string
	refreshes      int
}

func (t *profileTokenFake) Token(context.Context) (string, error) { return t.token, nil }
func (t *profileTokenFake) RefreshToken(context.Context) (string, error) {
	t.refreshes++
	return t.refresh, nil
}
func TestWeComContactProfileWriterClassifiesReceiptsWithoutPII(t *testing.T) {
	for name, test := range map[string]struct {
		body string
		want eer.Completion
	}{"executed": {`{"errcode":0}`, eer.CompletionExecuted}, "rejected": {`{"errcode":84061}`, eer.CompletionFinalFailed}} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/cgi-bin/externalcontact/remark" || r.URL.Query().Get("access_token") != "token" {
					t.Fatalf("request=%s", r.URL.String())
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			writer, err := NewWeComContactProfileClient(WeComContactProfileClientConfig{BaseURL: server.URL, HTTPClient: server.Client(), Token: &profileTokenFake{token: "token"}})
			if err != nil {
				t.Fatal(err)
			}
			got, err := writer.WriteContactProfile(context.Background(), wecomport.ContactProfileWriteRequest{CorpID: "corp", StaffUserID: "staff", ExternalUserID: "external", Remark: "VIP"})
			if err != nil || got.Completion != test.want || !got.BusinessCallDispatched || !got.RealExternalCallExecuted {
				t.Fatalf("got=%+v err=%v", got, err)
			}
		})
	}
}
