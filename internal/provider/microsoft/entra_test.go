package microsoft

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
)

func TestResolveUsersTargetsDistinctUsersAndConfiguredGroups(t *testing.T) {
	var mutex sync.Mutex
	groupCalls := make(map[string]int)
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		parts := strings.Split(strings.Trim(request.URL.Path, "/"), "/")
		if len(parts) < 3 || parts[0] != "v1.0" || parts[1] != "users" {
			return nil, fmt.Errorf("unexpected path %s", request.URL.Path)
		}
		identifier := parts[2]
		if len(parts) == 3 {
			if got := request.URL.Query().Get("$select"); got != "id,userPrincipalName,mailNickname,displayName,department" {
				t.Errorf("$select = %q", got)
			}
			return jsonResponse(http.StatusOK, map[string]any{
				"id":                "object-" + identifier,
				"userPrincipalName": identifier + "@example.com",
				"mailNickname":      identifier,
				"displayName":       "Fixture " + identifier,
				"department":        "Fixture",
			}), nil
		}
		if len(parts) != 4 || parts[3] != "checkMemberGroups" {
			return nil, fmt.Errorf("unexpected user path %s", request.URL.Path)
		}
		var body struct {
			GroupIDs []string `json:"groupIds"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode checkMemberGroups body: %v", err)
		}
		if len(body.GroupIDs) > checkMemberGroupsLimit {
			t.Errorf("groupIds count = %d, want at most %d", len(body.GroupIDs), checkMemberGroupsLimit)
		}
		mutex.Lock()
		groupCalls[identifier]++
		mutex.Unlock()
		memberships := []string{}
		userKey := strings.TrimPrefix(identifier, "object-")
		for _, groupID := range body.GroupIDs {
			if userKey == "fixture-a" && groupID == "group-01" {
				memberships = append(memberships, groupID)
			}
			if userKey == "fixture-b" && groupID == "group-21" {
				memberships = append(memberships, groupID)
			}
		}
		return jsonResponse(http.StatusOK, map[string]any{"value": memberships}), nil
	})
	client := newTestClient(t, transport)

	groupAliases := map[string][]string{
		"first":  {"group-01"},
		"shared": {"group-01", "group-21"},
		"last":   {"group-21"},
	}
	for number := 2; number <= 20; number++ {
		groupAliases[fmt.Sprintf("unused-%02d", number)] = []string{fmt.Sprintf("group-%02d", number)}
	}
	users, err := client.ResolveUsers(
		context.Background(),
		[]string{"fixture-b", "fixture-a", "fixture-a"},
		groupAliases,
		2,
	)
	if err != nil {
		t.Fatalf("ResolveUsers() error = %v", err)
	}
	if got, want := len(users), 2; got != want {
		t.Fatalf("resolved users = %d, want %d", got, want)
	}
	if got, want := users["fixture-a"].Groups, []string{"first", "shared"}; !reflect.DeepEqual(got, want) {
		t.Errorf("fixture-a groups = %#v, want %#v", got, want)
	}
	if got, want := users["fixture-b"].Groups, []string{"last", "shared"}; !reflect.DeepEqual(got, want) {
		t.Errorf("fixture-b groups = %#v, want %#v", got, want)
	}
	if got, want := users["fixture-a"].MailNickname, "fixture-a"; got != want {
		t.Errorf("mail nickname = %q, want %q", got, want)
	}

	mutex.Lock()
	defer mutex.Unlock()
	identifiers := make([]string, 0, len(groupCalls))
	for identifier, calls := range groupCalls {
		identifiers = append(identifiers, identifier)
		if calls != 2 {
			t.Errorf("%s group calls = %d, want 2 chunks", identifier, calls)
		}
	}
	sort.Strings(identifiers)
	if got, want := identifiers, []string{"object-fixture-a", "object-fixture-b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("group identifiers = %#v, want %#v", got, want)
	}
}

func TestResolveUsersTreatsMissingInventoryIdentityAsAbsent(t *testing.T) {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if strings.Contains(request.URL.Path, "missing-fixture") {
			return jsonResponse(http.StatusNotFound, map[string]any{
				"error": map[string]string{"code": "Request_ResourceNotFound", "message": "not found"},
			}), nil
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"id":                "object-unit",
			"userPrincipalName": "unit@example.invalid",
			"mailNickname":      "unit",
		}), nil
	})
	client := newTestClient(t, transport)

	users, err := client.ResolveUsers(
		context.Background(),
		[]string{"missing-fixture", "unit"},
		nil,
		1,
	)
	if err != nil {
		t.Fatalf("ResolveUsers() error = %v", err)
	}
	if got, want := len(users), 2; got != want {
		t.Fatalf("resolved users = %d, want %d", got, want)
	}
	if users["missing-fixture"].Present {
		t.Error("missing identity is present")
	}
	if !users["unit"].Present {
		t.Error("existing identity is absent")
	}
}
