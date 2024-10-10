package azure

type Subscription struct {
	Id       string `json:"id"`
	Name     string `json:"name"`
	TenantId string `json:"tenantId"`
	// The tenant under which the user has access to the subscription.
	UserAccessTenantId string `json:"userAccessTenantId"`
	IsDefault          bool   `json:"isDefault,omitempty"`
}

type Location struct {
	// The name of the location (e.g. "westus2")
	Name string `json:"name"`
	// The human friendly name of the location (e.g. "West US 2")
	DisplayName string `json:"displayName"`
	// The human friendly name of the location, prefixed with a
	// region name (e.g "(US) West US 2")
	RegionalDisplayName string `json:"regionalDisplayName"`
}

type ResourceGroup struct {
	Id       string `json:"id"`
	Name     string `json:"name"`
	Location string `json:"location"`
}

type Resource struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}
