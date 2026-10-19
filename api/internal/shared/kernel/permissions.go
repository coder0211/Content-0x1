package kernel

// Permission codes (SPEC-01 §2). Every authorization check uses these.
const (
	PermUserManage     = "user.manage"
	PermProviderManage = "provider.manage"
	PermProviderView   = "provider.view"
	PermBudgetManage   = "budget.manage"
	PermBrandManage    = "brand.manage"
	PermBrandView      = "brand.view"
	PermCampaignManage = "campaign.manage"
	PermGenerateRun    = "generate.run"
	PermAssetReview    = "asset.review"
	PermAssetDownload  = "asset.download"
	PermUsageView      = "usage.view"
	PermAuditView      = "audit.view"
)

// Role codes.
const (
	RoleAdmin          = "admin"
	RoleMarketingLead  = "marketing_lead"
	RoleContentCreator = "content_creator"
	RoleReviewer       = "reviewer"
)

// AllPermissions lists every permission code in matrix order.
var AllPermissions = []string{
	PermUserManage, PermProviderManage, PermProviderView, PermBudgetManage,
	PermBrandManage, PermBrandView, PermCampaignManage, PermGenerateRun,
	PermAssetReview, PermAssetDownload, PermUsageView, PermAuditView,
}

// AllRoles lists every role code.
var AllRoles = []string{RoleAdmin, RoleMarketingLead, RoleContentCreator, RoleReviewer}

// RolePermissions is the RBAC matrix of SPEC-01 §2. The database
// (role_permissions, seeded by migration 0011) is the runtime source of
// truth; a test keeps the two in sync.
var RolePermissions = map[string][]string{
	RoleAdmin: AllPermissions,
	RoleMarketingLead: {
		PermProviderView, PermBudgetManage, PermBrandManage, PermBrandView,
		PermCampaignManage, PermGenerateRun, PermAssetReview, PermAssetDownload, PermUsageView,
	},
	RoleContentCreator: {PermBrandView, PermCampaignManage, PermGenerateRun, PermAssetDownload},
	RoleReviewer:       {PermBrandView, PermAssetReview, PermAssetDownload},
}

// IsKnownRole reports whether code is one of AllRoles.
func IsKnownRole(code string) bool {
	_, ok := RolePermissions[code]
	return ok
}
