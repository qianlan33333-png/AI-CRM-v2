package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestTagCatalogClientReadsFrozenCorpTagDirectory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/cgi-bin/externalcontact/get_corp_tag_list" ||
			request.URL.Query().Get("access_token") != "token-safe" || request.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("request=%s %s?%s content-type=%q", request.Method, request.URL.Path, request.URL.RawQuery, request.Header.Get("Content-Type"))
		}
		body, err := io.ReadAll(request.Body)
		if err != nil || string(body) != "{}" {
			t.Fatalf("body=%q err=%v", body, err)
		}
		_, _ = writer.Write([]byte(`{"errcode":0,"tag_group":[{"group_id":"group-1","group_name":"Lifecycle","order":2,"tag":[{"id":"tag-1","name":"Warm","order":3},{"id":"tag-deleted","name":"Old","order":4,"deleted":true}]},{"group_id":"group-2","group_name":"Source","order":5,"tag":[]}]}`))
	}))
	defer server.Close()
	client, err := NewTagCatalogClient(TagCatalogClientConfig{BaseURL: server.URL, HTTPClient: server.Client(), TokenProvider: staticTokenProvider{token: AccessToken{value: "token-safe"}}})
	if err != nil {
		t.Fatal(err)
	}

	got, err := client.ListCorpTags(context.Background())
	want := CorpTagCatalog{Groups: []CorpTagGroup{
		{ProviderGroupID: "group-1", Name: "Lifecycle", Order: 2, Tags: []CorpTag{{ProviderTagID: "tag-1", Name: "Warm", Order: 3}}},
		{ProviderGroupID: "group-2", Name: "Source", Order: 5, Tags: []CorpTag{}},
	}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("ListCorpTags() = %#v, %v; want %#v", got, err, want)
	}
}

func TestTagCatalogClientFailsClosedForMalformedOrRejectedDirectory(t *testing.T) {
	for _, response := range []string{
		`{"errcode":0}`,
		`{"errcode":0,"tag_group":[{"group_id":"group-1","group_name":"Lifecycle","tag":[{"id":"tag-1","name":"Warm"},{"id":"tag-1","name":"Duplicate"}]}]}`,
		`{"errcode":0,"tag_group":[{"group_id":"group-1","group_name":"Lifecycle"},{"group_id":"group-1","group_name":"Duplicate"}]}`,
		`{"errcode":0,"tag_group":[{"group_id":"group-1","group_name":"Lifecycle","tag":[{"id":"tag-1","name":"bad\nname"}]}]}`,
		`{"errcode":48002,"errmsg":"forbidden"}`,
		`not-json`,
	} {
		t.Run(response, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				_, _ = writer.Write([]byte(response))
			}))
			defer server.Close()
			client, err := NewTagCatalogClient(TagCatalogClientConfig{BaseURL: server.URL, HTTPClient: server.Client(), TokenProvider: staticTokenProvider{token: AccessToken{value: "token-safe"}}})
			if err != nil {
				t.Fatal(err)
			}
			got, err := client.ListCorpTags(context.Background())
			if got.Groups != nil || (!errors.Is(err, ErrUnexpectedResponse) && !errors.Is(err, ErrUpstream)) {
				t.Fatalf("ListCorpTags() = %#v, %v", got, err)
			}
		})
	}
	if _, err := NewTagCatalogClient(TagCatalogClientConfig{}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("NewTagCatalogClient() error = %v", err)
	}
}
