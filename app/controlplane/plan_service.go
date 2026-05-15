package controlplane

import "slices"

import "context"

type Plan string

const (
	PlanFree       Plan = "free"
	PlanPro        Plan = "pro"
	PlanEnterprise Plan = "enterprise"
)

type Feature string

const (
	FeatureSlowPath       Feature = "slow_path"
	FeatureVision         Feature = "vision"
	FeatureCustomRules    Feature = "custom_rules"
	FeatureMultiWorkspace Feature = "multi_workspace"
	FeaturePriorityQueue  Feature = "priority_queue"
	FeatureExport         Feature = "export"
	FeatureAppeals        Feature = "appeals"
)

type PlanLimits struct {
	SpamChecksPerHour   int
	SlowPathPerHour     int
	MaxWorkspaces       int
	MaxRuleSets         int
	HistoryRetentionHrs int
}

type PlanDefinition struct {
	Name     Plan
	Features []Feature
	Limits   PlanLimits
}

var planCatalog = map[Plan]PlanDefinition{
	PlanFree: {
		Name:     PlanFree,
		Features: []Feature{FeatureCustomRules, FeatureAppeals},
		Limits: PlanLimits{
			SpamChecksPerHour:   1000,
			SlowPathPerHour:     0,
			MaxWorkspaces:       1,
			MaxRuleSets:         1,
			HistoryRetentionHrs: 168,
		},
	},
	PlanPro: {
		Name:     PlanPro,
		Features: []Feature{FeatureSlowPath, FeatureCustomRules, FeatureMultiWorkspace, FeatureExport, FeatureAppeals},
		Limits: PlanLimits{
			SpamChecksPerHour:   10000,
			SlowPathPerHour:     500,
			MaxWorkspaces:       5,
			MaxRuleSets:         10,
			HistoryRetentionHrs: 720,
		},
	},
	PlanEnterprise: {
		Name: PlanEnterprise,
		Features: []Feature{
			FeatureSlowPath, FeatureVision, FeatureCustomRules, FeatureMultiWorkspace,
			FeaturePriorityQueue, FeatureExport, FeatureAppeals,
		},
		Limits: PlanLimits{
			SpamChecksPerHour:   0,
			SlowPathPerHour:     0,
			MaxWorkspaces:       0,
			MaxRuleSets:         0,
			HistoryRetentionHrs: 0,
		},
	},
}

type TenantPlanStore interface {
	GetPlan(ctx context.Context, tenantID string) (Plan, error)
	SetPlan(ctx context.Context, tenantID, planName string) error
}

type PlanService struct {
	store TenantPlanStore
}

func NewPlanService(store TenantPlanStore) *PlanService {
	return &PlanService{store: store}
}

func (s *PlanService) GetPlan(ctx context.Context, tenantID string) (PlanDefinition, error) {
	planName, err := s.store.GetPlan(ctx, tenantID)
	if err != nil {
		planName = PlanFree
	}
	def, ok := planCatalog[planName]
	if !ok {
		return planCatalog[PlanFree], nil
	}
	return def, nil
}

func (s *PlanService) HasFeature(ctx context.Context, tenantID string, feature Feature) (bool, error) {
	def, err := s.GetPlan(ctx, tenantID)
	if err != nil {
		return false, err
	}
	if slices.Contains(def.Features, feature) {
		return true, nil
	}
	return false, nil
}

func (s *PlanService) SetPlan(ctx context.Context, tenantID, planName string) error {
	return s.store.SetPlan(ctx, tenantID, planName)
}

func GetPlanDefinition(name Plan) (PlanDefinition, bool) {
	def, ok := planCatalog[name]
	return def, ok
}
