package microsoft

import (
	"context"
	"fmt"
	"slices"
	"sort"

	graphusers "github.com/microsoftgraph/msgraph-sdk-go/users"

	"github.com/woodleighschool/onomazo/internal/domain"
)

const checkMemberGroupsLimit = 20

type resolvedUser struct {
	identifier string
	user       domain.User
	err        error
}

// ResolveUsers fetches only the distinct inventory-associated users and configured group aliases.
func (c *Client) ResolveUsers(
	ctx context.Context,
	identifiers []string,
	groupAliases map[string][]string,
	concurrency int,
) (map[string]domain.User, error) {
	if c == nil || c.graph == nil {
		return nil, fmt.Errorf("graph client is required")
	}
	if concurrency <= 0 {
		return nil, fmt.Errorf("concurrency must be greater than zero")
	}
	identifiers = uniqueSorted(identifiers)
	if len(identifiers) == 0 {
		return map[string]domain.User{}, nil
	}
	groupIDs, aliasesByID := prepareGroupAliases(groupAliases)

	jobs := make(chan string, len(identifiers))
	results := make(chan resolvedUser, len(identifiers))
	for _, identifier := range identifiers {
		jobs <- identifier
	}
	close(jobs)
	workerCount := min(concurrency, len(identifiers))
	for range workerCount {
		go func() {
			for identifier := range jobs {
				user, err := c.resolveUser(ctx, identifier, groupIDs, aliasesByID)
				results <- resolvedUser{identifier: identifier, user: user, err: err}
			}
		}()
	}

	users := make(map[string]domain.User, len(identifiers))
	errorsByIdentifier := make(map[string]error)
	for range identifiers {
		result := <-results
		if result.err != nil {
			errorsByIdentifier[result.identifier] = result.err
			continue
		}
		users[result.identifier] = result.user
	}
	if len(errorsByIdentifier) != 0 {
		for _, identifier := range identifiers {
			if err := errorsByIdentifier[identifier]; err != nil {
				return nil, fmt.Errorf("resolve Entra user: %w", err)
			}
		}
	}
	return users, nil
}

func (c *Client) resolveUser(
	ctx context.Context,
	identifier string,
	groupIDs []string,
	aliasesByID map[string][]string,
) (domain.User, error) {
	response, err := c.graph.Users().ByUserId(identifier).Get(
		ctx,
		&graphusers.UserItemRequestBuilderGetRequestConfiguration{
			QueryParameters: &graphusers.UserItemRequestBuilderGetQueryParameters{
				Select: []string{
					"id",
					"userPrincipalName",
					"mailNickname",
					"displayName",
					"department",
				},
			},
		},
	)
	if err != nil {
		return domain.User{}, err
	}
	if response == nil {
		return domain.User{}, fmt.Errorf("graph returned no user")
	}
	groups, err := c.checkGroupAliases(ctx, dereference(response.GetId()), groupIDs, aliasesByID)
	if err != nil {
		return domain.User{}, err
	}
	return domain.User{
		Present:           true,
		ID:                dereference(response.GetId()),
		MailNickname:      dereference(response.GetMailNickname()),
		UserPrincipalName: dereference(response.GetUserPrincipalName()),
		DisplayName:       dereference(response.GetDisplayName()),
		Department:        dereference(response.GetDepartment()),
		Groups:            groups,
	}, nil
}

func (c *Client) checkGroupAliases(
	ctx context.Context,
	userID string,
	groupIDs []string,
	aliasesByID map[string][]string,
) ([]string, error) {
	if len(groupIDs) == 0 {
		return nil, nil
	}
	if userID == "" {
		return nil, fmt.Errorf("resolved Entra user has no ID")
	}
	aliases := make(map[string]struct{})
	for start := 0; start < len(groupIDs); start += checkMemberGroupsLimit {
		end := min(start+checkMemberGroupsLimit, len(groupIDs))
		body := graphusers.NewItemCheckMemberGroupsPostRequestBody()
		body.SetGroupIds(groupIDs[start:end])
		response, err := c.graph.Users().ByUserId(userID).CheckMemberGroups().PostAsCheckMemberGroupsPostResponse(
			ctx,
			body,
			nil,
		)
		if err != nil {
			return nil, fmt.Errorf("check Entra groups: %w", err)
		}
		if response == nil {
			return nil, fmt.Errorf("check Entra groups: Graph returned no response")
		}
		for _, groupID := range response.GetValue() {
			for _, alias := range aliasesByID[groupID] {
				aliases[alias] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(aliases))
	for alias := range aliases {
		result = append(result, alias)
	}
	sort.Strings(result)
	return result, nil
}

func prepareGroupAliases(groupAliases map[string][]string) ([]string, map[string][]string) {
	aliasesByID := make(map[string][]string)
	for alias, groupIDs := range groupAliases {
		for _, groupID := range groupIDs {
			aliasesByID[groupID] = append(aliasesByID[groupID], alias)
		}
	}
	groupIDs := make([]string, 0, len(aliasesByID))
	for groupID, aliases := range aliasesByID {
		sort.Strings(aliases)
		aliasesByID[groupID] = slices.Compact(aliases)
		groupIDs = append(groupIDs, groupID)
	}
	sort.Strings(groupIDs)
	return groupIDs, aliasesByID
}

func uniqueSorted(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return slices.Compact(result)
}

func dereference(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func int32Pointer(value int32) *int32 {
	return &value
}
