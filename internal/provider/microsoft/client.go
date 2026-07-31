package microsoft

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
)

const defaultGraphBaseURL = "https://graph.microsoft.com/v1.0"

var graphScopes = []string{"https://graph.microsoft.com/.default"}

// Config contains one Microsoft Graph application connection.
type Config struct {
	TenantID     string
	ClientID     string
	ClientSecret string
	BaseURL      string
}

// Client shares one credential and Graph SDK client across Intune and Entra operations.
type Client struct {
	graph       *msgraphsdk.GraphServiceClient
	betaBaseURL string
}

// NewClient creates an application-authenticated Microsoft Graph client.
func NewClient(config Config) (*Client, error) {
	if config.TenantID == "" || config.ClientID == "" || config.ClientSecret == "" {
		return nil, fmt.Errorf("tenant ID, client ID, and client secret are required")
	}
	baseURL := strings.TrimRight(config.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultGraphBaseURL
	}
	betaBaseURL, err := graphVersionURL(baseURL, "beta")
	if err != nil {
		return nil, err
	}

	credential, err := azidentity.NewClientSecretCredential(
		config.TenantID,
		config.ClientID,
		config.ClientSecret,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("create Microsoft credential: %w", err)
	}
	graph, err := msgraphsdk.NewGraphServiceClientWithCredentials(credential, graphScopes)
	if err != nil {
		return nil, fmt.Errorf("create Microsoft Graph client: %w", err)
	}
	graph.GetAdapter().SetBaseUrl(baseURL)
	return newClient(graph, betaBaseURL), nil
}

func newClient(graph *msgraphsdk.GraphServiceClient, betaBaseURL string) *Client {
	return &Client{graph: graph, betaBaseURL: strings.TrimRight(betaBaseURL, "/")}
}

func graphVersionURL(baseURL, version string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("graph base URL must be absolute")
	}
	parsed.Path = "/" + version
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}
