package zitadel

// Organization types
type CreateOrgRequest struct {
	Name string `json:"name"`
}

type OrgResponse struct {
	Org OrgDetail `json:"org"`
}

type OrgDetail struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}

// Project types
type CreateProjectRequest struct {
	Name                 string `json:"name"`
	ProjectRoleAssertion bool   `json:"projectRoleAssertion"`
	ProjectRoleCheck     bool   `json:"projectRoleCheck"`
	HasProjectCheck      bool   `json:"hasProjectCheck"`
}

type ProjectResponse struct {
	ID string `json:"id"`
}

type UpdateProjectRequest struct {
	Name                 string `json:"name"`
	ProjectRoleAssertion bool   `json:"projectRoleAssertion"`
	ProjectRoleCheck     bool   `json:"projectRoleCheck"`
	HasProjectCheck      bool   `json:"hasProjectCheck"`
}

// Application types
type CreateOIDCAppRequest struct {
	Name                   string   `json:"name"`
	RedirectUris           []string `json:"redirectUris"`
	PostLogoutRedirectUris []string `json:"postLogoutRedirectUris,omitempty"`
	ResponseTypes          []string `json:"responseTypes"`
	GrantTypes             []string `json:"grantTypes"`
	AppType                string   `json:"appType"`
	AuthMethodType         string   `json:"authMethodType"`
	AccessTokenType        string   `json:"accessTokenType,omitempty"`
	DevMode                bool     `json:"devMode"`
}

type CreateAPIAppRequest struct {
	Name           string `json:"name"`
	AuthMethodType string `json:"authMethodType"`
}

type CreateSAMLAppRequest struct {
	Name        string `json:"name"`
	MetadataXml string `json:"metadataXml,omitempty"`
	MetadataUrl string `json:"metadataUrl,omitempty"`
}

type AppResponse struct {
	AppID        string `json:"appId"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret,omitempty"`
}

// User types
type CreateHumanUserRequest struct {
	UserName string        `json:"userName"`
	Profile  UserProfile   `json:"profile"`
	Email    UserEmail     `json:"email"`
	Password *UserPassword `json:"password,omitempty"`
}

type UserProfile struct {
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
}

type UserEmail struct {
	Email           string `json:"email"`
	IsEmailVerified bool   `json:"isEmailVerified"`
}

type UserPassword struct {
	Password string `json:"password"`
}

type CreateMachineUserRequest struct {
	UserName    string `json:"userName"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type UserResponse struct {
	UserID string `json:"userId"`
}

type PATResponse struct {
	TokenID string `json:"tokenId"`
	Token   string `json:"token"`
}

// Role types
type BulkAddRolesRequest struct {
	Roles []RoleEntry `json:"roles"`
}

type RoleEntry struct {
	Key         string `json:"key"`
	DisplayName string `json:"displayName"`
	Group       string `json:"group,omitempty"`
}

type ListRolesResponse struct {
	Result []RoleResult `json:"result"`
}

type RoleResult struct {
	Key         string `json:"key"`
	DisplayName string `json:"displayName"`
	Group       string `json:"group"`
}

// Generic details response used by many Zitadel API calls
type DetailsResponse struct {
	Details Details `json:"details"`
}

type Details struct {
	Sequence      string `json:"sequence"`
	ResourceOwner string `json:"resourceOwner"`
}
