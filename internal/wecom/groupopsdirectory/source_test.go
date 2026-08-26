package groupopsdirectory

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
	wecomclient "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/client"
)

func TestListOwnedGroupsReadsOwnerScopedPagesAndExactGroups(t *testing.T) {
	var lists atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/cgi-bin/gettoken":
			_, _ = writer.Write([]byte(`{"errcode":0,"access_token":"token-safe","expires_in":7200}`))
		case "/cgi-bin/externalcontact/groupchat/list":
			if request.Method != http.MethodPost || request.URL.Query().Get("access_token") != "token-safe" {
				t.Fatalf("list request = %s %s", request.Method, request.URL.RawQuery)
			}
			var payload struct {
				OwnerFilter struct {
					UserIDs []string `json:"userid_list"`
				} `json:"owner_filter"`
				StatusFilter int32  `json:"status_filter"`
				Cursor       string `json:"cursor"`
				Limit        int32  `json:"limit"`
			}
			if json.NewDecoder(request.Body).Decode(&payload) != nil || !reflect.DeepEqual(payload.OwnerFilter.UserIDs, []string{"owner-7"}) || payload.StatusFilter != 0 {
				t.Fatalf("list payload = %#v", payload)
			}
			switch lists.Add(1) {
			case 1:
				if payload.Cursor != "" || payload.Limit != 2 {
					t.Fatalf("first list page = %#v", payload)
				}
				_, _ = writer.Write([]byte(`{"errcode":0,"group_chat_list":[{"chat_id":"chat-1"}],"next_cursor":"cursor-2"}`))
			case 2:
				if payload.Cursor != "cursor-2" || payload.Limit != 2 {
					t.Fatalf("second list page = %#v", payload)
				}
				_, _ = writer.Write([]byte(`{"errcode":0,"group_chat_list":[{"chat_id":"chat-2"}]}`))
			default:
				t.Fatalf("unexpected list page")
			}
		case "/cgi-bin/externalcontact/groupchat/get":
			if request.Method != http.MethodPost || request.URL.Query().Get("access_token") != "token-safe" {
				t.Fatalf("get request = %s %s", request.Method, request.URL.RawQuery)
			}
			var payload struct {
				ChatID   string `json:"chat_id"`
				NeedName int32  `json:"need_name"`
			}
			if json.NewDecoder(request.Body).Decode(&payload) != nil || payload.NeedName != 0 {
				t.Fatalf("get payload = %#v", payload)
			}
			switch payload.ChatID {
			case "chat-1":
				_, _ = writer.Write([]byte(`{"errcode":0,"group_chat":{"chat_id":"chat-1","name":"一号群","owner":"owner-7","member_list":[{"userid":"owner-7"},{"userid":"member-1"}]}}`))
			case "chat-2":
				_, _ = writer.Write([]byte(`{"errcode":0,"group_chat":{"chat_id":"chat-2","name":"二号群","owner":"owner-7","member_list":[{"userid":"owner-7"}]}}`))
			default:
				t.Fatalf("chat id = %q", payload.ChatID)
			}
		default:
			t.Fatalf("unexpected request %s", request.URL.Path)
		}
	}))
	defer server.Close()

	source := newSource(t, server, ownerResolverFunc(func(context.Context, int64) (string, error) { return "owner-7", nil }), activeStaffFunc(nil))
	snapshot, err := source.ListOwnedGroups(context.Background(), 7, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := []groupopsport.GroupDirectoryItem{
		{ChatReference: "chat-1", OwnerStaffID: 7, DisplayName: "一号群", MemberCount: 2},
		{ChatReference: "chat-2", OwnerStaffID: 7, DisplayName: "二号群", MemberCount: 1},
	}
	if !snapshot.Complete || !reflect.DeepEqual(snapshot.Items, want) {
		t.Fatalf("ListOwnedGroups() = %#v, want %#v", snapshot, want)
	}
}

func TestListOwnedGroupsFailsClosedForWrongOwnerAndDuplicateChats(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		list    string
		group   string
		wantErr error
	}{
		{name: "owner mismatch", list: `{"errcode":0,"group_chat_list":[{"chat_id":"chat-1"}]}`, group: `{"errcode":0,"group_chat":{"chat_id":"chat-1","name":"群","owner":"other","member_list":[{"userid":"other"}]}}`, wantErr: ErrUnexpectedResponse},
		{name: "duplicate chat", list: `{"errcode":0,"group_chat_list":[{"chat_id":"chat-1"},{"chat_id":"chat-1"}]}`, wantErr: ErrUnexpectedResponse},
		{name: "empty member", list: `{"errcode":0,"group_chat_list":[{"chat_id":"chat-1"}]}`, group: `{"errcode":0,"group_chat":{"chat_id":"chat-1","name":"群","owner":"owner-7","member_list":[]}}`, wantErr: ErrUnexpectedResponse},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/cgi-bin/gettoken":
					_, _ = writer.Write([]byte(`{"errcode":0,"access_token":"token-safe","expires_in":7200}`))
				case "/cgi-bin/externalcontact/groupchat/list":
					_, _ = writer.Write([]byte(testCase.list))
				case "/cgi-bin/externalcontact/groupchat/get":
					_, _ = writer.Write([]byte(testCase.group))
				default:
					t.Fatalf("unexpected request %s", request.URL.Path)
				}
			}))
			defer server.Close()
			source := newSource(t, server, ownerResolverFunc(func(context.Context, int64) (string, error) { return "owner-7", nil }), activeStaffFunc(nil))
			if _, err := source.ListOwnedGroups(context.Background(), 7, 2); !errors.Is(err, testCase.wantErr) {
				t.Fatalf("ListOwnedGroups() error = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestListGroupChatIDsReadsMoreThanOneThousandGroupsBeforeSnapshotCompletion(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/cgi-bin/gettoken":
			_, _ = writer.Write([]byte(`{"errcode":0,"access_token":"token-safe","expires_in":7200}`))
		case "/cgi-bin/externalcontact/groupchat/list":
			requests++
			itemCount := 1000
			nextCursor := "more"
			if requests == 2 {
				itemCount = 1
				nextCursor = ""
			}
			items := make([]map[string]string, itemCount)
			for index := range items {
				items[index] = map[string]string{"chat_id": "chat-" + strconv.Itoa((requests-1)*1000+index+1)}
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"errcode": 0, "group_chat_list": items, "next_cursor": nextCursor})
		default:
			t.Fatalf("unexpected request %s", request.URL.Path)
		}
	}))
	defer server.Close()

	source := newSource(t, server, ownerResolverFunc(func(context.Context, int64) (string, error) { return "owner-7", nil }), activeStaffFunc(nil))
	items, err := source.listGroupChatIDs(context.Background(), "owner-7", 1000)
	if err != nil || requests != 2 || len(items) != 1001 {
		t.Fatalf("listGroupChatIDs() = %d items requests=%d err=%v", len(items), requests, err)
	}
}

func TestRefreshOperationMembersRefreshesExpiredTokenAndReturnsStableIntersection(t *testing.T) {
	var grants atomic.Int32
	var reads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/cgi-bin/gettoken":
			if grants.Add(1) == 1 {
				_, _ = writer.Write([]byte(`{"errcode":0,"access_token":"stale-token","expires_in":7200}`))
				return
			}
			_, _ = writer.Write([]byte(`{"errcode":0,"access_token":"fresh-token","expires_in":7200}`))
		case "/cgi-bin/externalcontact/get_follow_user_list":
			if request.Method != http.MethodGet {
				t.Fatalf("method = %s", request.Method)
			}
			switch request.URL.Query().Get("access_token") {
			case "stale-token":
				_, _ = writer.Write([]byte(`{"errcode":42001,"errmsg":"expired"}`))
			case "fresh-token":
				reads.Add(1)
				_, _ = writer.Write([]byte(`{"errcode":0,"follow_user":["staff-b","staff-a","staff-b","unmanaged"]}`))
			default:
				t.Fatalf("token = %q", request.URL.Query().Get("access_token"))
			}
		default:
			t.Fatalf("unexpected request %s", request.URL.Path)
		}
	}))
	defer server.Close()

	source := newSource(t, server, ownerResolverFunc(nil), activeStaffFunc(func(context.Context) ([]ActiveStaff, error) {
		return []ActiveStaff{{WeComUserID: "staff-b", DisplayName: "乙"}, {WeComUserID: "staff-a", DisplayName: "甲"}, {WeComUserID: "staff-b", DisplayName: "乙"}, {WeComUserID: "not-following", DisplayName: "未关注"}}, nil
	}))
	items, err := source.RefreshOperationMembers(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	want := []groupopsport.OperationMember{{SenderUserID: "staff-a", DisplayName: "甲"}, {SenderUserID: "staff-b", DisplayName: "乙"}}
	if !reflect.DeepEqual(items, want) {
		t.Fatalf("RefreshOperationMembers() = %#v, want %#v", items, want)
	}
	if grants.Load() != 2 || reads.Load() != 1 {
		t.Fatalf("grants=%d reads=%d, want 2 and 1", grants.Load(), reads.Load())
	}
}

func TestRefreshOperationMembersFailsClosedForMalformedActiveStaff(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/cgi-bin/gettoken":
			_, _ = writer.Write([]byte(`{"errcode":0,"access_token":"token-safe","expires_in":7200}`))
		case "/cgi-bin/externalcontact/get_follow_user_list":
			_, _ = writer.Write([]byte(`{"errcode":0,"follow_user":["staff-a"]}`))
		default:
			t.Fatalf("unexpected request %s", request.URL.Path)
		}
	}))
	defer server.Close()
	source := newSource(t, server, ownerResolverFunc(nil), activeStaffFunc(func(context.Context) ([]ActiveStaff, error) {
		return []ActiveStaff{{WeComUserID: "staff-a", DisplayName: "  空白"}}, nil
	}))
	if _, err := source.RefreshOperationMembers(context.Background(), 10); !errors.Is(err, ErrUnexpectedResponse) {
		t.Fatalf("RefreshOperationMembers() error = %v, want malformed local staff rejection", err)
	}
}

func TestRefreshOperationMembersDoesNotRefreshNonExpiredProviderErrors(t *testing.T) {
	var grants atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/cgi-bin/gettoken":
			grants.Add(1)
			_, _ = writer.Write([]byte(`{"errcode":0,"access_token":"token-safe","expires_in":7200}`))
		case "/cgi-bin/externalcontact/get_follow_user_list":
			_, _ = writer.Write([]byte(`{"errcode":48002,"errmsg":"forbidden"}`))
		default:
			t.Fatalf("unexpected request %s", request.URL.Path)
		}
	}))
	defer server.Close()
	source := newSource(t, server, ownerResolverFunc(nil), activeStaffFunc(nil))
	if _, err := source.RefreshOperationMembers(context.Background(), 10); !errors.Is(err, ErrUpstream) {
		t.Fatalf("RefreshOperationMembers() error = %v, want upstream rejection", err)
	}
	if grants.Load() != 1 {
		t.Fatalf("token grants = %d, want no refresh", grants.Load())
	}
}

func newSource(t *testing.T, server *httptest.Server, owner OwnerStaffResolver, active ActiveStaffDirectory) *Source {
	t.Helper()
	credentials, err := wecomclient.NewCredentials("corp-fixture", "secret-fixture")
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := wecomclient.NewTokenProvider(wecomclient.TokenProviderConfig{BaseURL: server.URL, Credentials: credentials, HTTPClient: server.Client(), Now: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	source, err := New(Config{BaseURL: server.URL, HTTPClient: server.Client(), Token: tokens, OwnerStaff: owner, ActiveStaff: active})
	if err != nil {
		t.Fatal(err)
	}
	return source
}

type ownerResolverFunc func(context.Context, int64) (string, error)

func (resolver ownerResolverFunc) ResolveActiveWeComUserID(ctx context.Context, staffID int64) (string, error) {
	if resolver == nil {
		return "", errors.New("owner resolver unavailable")
	}
	return resolver(ctx, staffID)
}

type activeStaffFunc func(context.Context) ([]ActiveStaff, error)

func (directory activeStaffFunc) ListActiveWeComStaff(ctx context.Context) ([]ActiveStaff, error) {
	if directory == nil {
		return []ActiveStaff{}, nil
	}
	return directory(ctx)
}
