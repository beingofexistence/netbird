package auth

import (
	"context"
	"fmt"
	"net/http"
	"runtime"

	"google.golang.org/grpc/codes"
	gstatus "google.golang.org/grpc/status"

	"github.com/netbirdio/netbird/client/internal"
)

// OAuthFlow represents an interface for authorization using different OAuth 2.0 flows
type OAuthFlow interface {
	RequestAuthInfo(ctx context.Context) (AuthFlowInfo, error)
	WaitToken(ctx context.Context, info AuthFlowInfo) (TokenInfo, error)
	GetClientID(ctx context.Context) string
}

// HTTPClient http client interface for API calls
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// AuthFlowInfo holds information for the OAuth 2.0  authorization flow
type AuthFlowInfo struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// Claims used when validating the access token
type Claims struct {
	Audience interface{} `json:"aud"`
}

// TokenInfo holds information of issued access token
type TokenInfo struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	UseIDToken   bool   `json:"-"`
}

// GetTokenToUse returns either the access or id token based on UseIDToken field
func (t TokenInfo) GetTokenToUse() string {
	if t.UseIDToken {
		return t.IDToken
	}
	return t.AccessToken
}

// NewOAuthFlow initializes and returns the appropriate OAuth flow based on the management configuration
//
// It starts by initializing the PKCE.If this process fails, it resorts to the Device Code Flow,
// and if that also fails, the authentication process is deemed unsuccessful
//
// On Linux distros without desktop environment support, it only tries to initialize the Device Code Flow
func NewOAuthFlow(ctx context.Context, config *internal.Config) (OAuthFlow, error) {
	if runtime.GOOS == "linux" && !isLinuxRunningDesktop() {
		return authenticateWithDeviceCodeFlow(ctx, config)
	}

	pkceFlow, err := authenticateWithPKCEFlow(ctx, config)
	if err != nil {
		// fallback to device code flow
		return authenticateWithDeviceCodeFlow(ctx, config)
	}
	return pkceFlow, nil
}

// authenticateWithPKCEFlow initializes the Proof Key for Code Exchange flow auth flow
func authenticateWithPKCEFlow(ctx context.Context, config *internal.Config) (OAuthFlow, error) {
	pkceFlowInfo, err := internal.GetPKCEAuthorizationFlowInfo(ctx, config.PrivateKey, config.ManagementURL)
	if err != nil {
		return nil, fmt.Errorf("getting pkce authorization flow info failed with error: %v", err)
	}
	return NewPKCEAuthorizationFlow(pkceFlowInfo.ProviderConfig)
}

// authenticateWithDeviceCodeFlow initializes the Device Code auth Flow
func authenticateWithDeviceCodeFlow(ctx context.Context, config *internal.Config) (OAuthFlow, error) {
	deviceFlowInfo, err := internal.GetDeviceAuthorizationFlowInfo(ctx, config.PrivateKey, config.ManagementURL)
	if err != nil {
		s, ok := gstatus.FromError(err)
		if ok && s.Code() == codes.NotFound {
			return nil, fmt.Errorf("no SSO provider returned from management. " +
				"Please proceed with setting up this device using setup keys " +
				"https://docs.netbird.io/how-to/register-machines-using-setup-keys")
		} else if ok && s.Code() == codes.Unimplemented {
			return nil, fmt.Errorf("the management server, %s, does not support SSO providers, "+
				"please update your server or use Setup Keys to login", config.ManagementURL)
		} else {
			return nil, fmt.Errorf("getting device authorization flow info failed with error: %v", err)
		}
	}

	return NewDeviceAuthorizationFlow(deviceFlowInfo.ProviderConfig)
}
