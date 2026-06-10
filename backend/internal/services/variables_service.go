package services

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/models"
	"spindle-edge/backend/internal/pipeline"
)

type VariablesService struct {
	repo           *database.Repository
	tags           *pipeline.TagManager
	edgeInstanceID string
}

type CreateVariableInput struct {
	VarID                  int64
	SourceType             string
	GatewayID              int
	SourceTopic            string
	SourcePath             string
	RawName                string
	ProjectID              *uint
	ProjectCode            string
	VarGroup               string
	VarName                string
	DisplayName            string
	DisplayNameEN          string
	DisplayNameJA          string
	JSONPath               string
	DataType               string
	Unit                   string
	DecimalPlaces          int
	ScaleFactor            float64
	OffsetVal              float64
	RWMode                 string
	Writable               bool
	WriteSourceID          int
	WritePath              string
	WriteDataType          string
	WriteMin               *float64
	WriteMax               *float64
	WriteEnum              string
	WriteRequiresAudit     *bool
	SuspiciousValue        *float64
	DebounceThreshold      *float64
	DebounceMS             int
	Deadband               float64
	DefaultAlarmEnabled    *bool
	DefaultLimitLL         *float64
	DefaultLimitL          *float64
	DefaultLimitH          *float64
	DefaultLimitHH         *float64
	DefaultLimitDeadband   float64
	DefaultViolationHoldMS int
	DefaultRecoverHoldMS   int
	Enabled                *bool
}

type UpdateVariableOptions struct {
	ApplyToRunning bool
}

type BulkRemapKIOProjectsInput struct {
	ProjectCount         int
	ProjectCodePrefix    string
	ProjectDisplayPrefix string
	ProjectENPrefix      string
	ProjectJAPrefix      string
	RawProjectPrefix     string
	VarGroup             string
	VarNamePrefix        string
	RemapVarName         *bool
	Enable               *bool
	DryRun               bool
}

type BulkRemapKIOProjectsResult struct {
	DryRun          bool                             `json:"dry_run"`
	ProjectCount    int                              `json:"project_count"`
	CreatedProjects int                              `json:"created_projects"`
	UpdatedProjects int                              `json:"updated_projects"`
	Matched         int                              `json:"matched"`
	Updated         int                              `json:"updated"`
	Skipped         int                              `json:"skipped"`
	Items           []BulkRemapKIOProjectsResultItem `json:"items"`
}

type BulkRemapKIOProjectsResultItem struct {
	VarID       int64  `json:"var_id"`
	VarIDText   string `json:"var_id_text"`
	RawName     string `json:"raw_name"`
	OldVarName  string `json:"old_var_name"`
	NewVarName  string `json:"new_var_name"`
	ProjectNo   int    `json:"project_no"`
	ProjectID   uint   `json:"project_id"`
	ProjectCode string `json:"project_code"`
	Action      string `json:"action"`
	Reason      string `json:"reason,omitempty"`
}

func NewVariablesService(repo *database.Repository, tags *pipeline.TagManager, edgeInstanceID ...string) *VariablesService {
	service := &VariablesService{repo: repo, tags: tags}
	if len(edgeInstanceID) > 0 {
		service.edgeInstanceID = strings.TrimSpace(edgeInstanceID[0])
	}
	return service
}

func (s *VariablesService) Snapshots(filter RealtimeVariableFilter) []models.TagSnapshot {
	return filteredTagSnapshots(s.tags, filter)
}

func (s *VariablesService) List(filter database.TagFilter) ([]models.TagConfig, error) {
	return s.repo.ListTags(filter)
}

func (s *VariablesService) BulkRemapKIOProjects(input BulkRemapKIOProjectsInput) (BulkRemapKIOProjectsResult, error) {
	input = normalizeBulkRemapKIOInput(input)
	result := BulkRemapKIOProjectsResult{DryRun: input.DryRun, ProjectCount: input.ProjectCount}
	projects := make(map[int]models.Project, input.ProjectCount)
	for projectNo := 1; projectNo <= input.ProjectCount; projectNo++ {
		project := models.Project{
			ProjectCode:   fmt.Sprintf("%s-%02d", input.ProjectCodePrefix, projectNo),
			SiteNo:        strconv.Itoa(projectNo),
			Name:          fmt.Sprintf("%s%d", input.ProjectDisplayPrefix, projectNo),
			DisplayName:   fmt.Sprintf("%s%d", input.ProjectDisplayPrefix, projectNo),
			DisplayNameEN: fmt.Sprintf("%s %d", input.ProjectENPrefix, projectNo),
			DisplayNameJA: fmt.Sprintf("%s%d", input.ProjectJAPrefix, projectNo),
			Enabled:       true,
		}
		if input.DryRun {
			projects[projectNo] = project
			continue
		}
		ensured, created, updated, err := s.repo.EnsureProjectByCode(project)
		if err != nil {
			return result, err
		}
		if created {
			result.CreatedProjects++
		}
		if updated {
			result.UpdatedProjects++
		}
		projects[projectNo] = ensured
	}

	tags, err := s.repo.ListTags(database.TagFilter{SourceType: models.TagSourceMQTT})
	if err != nil {
		return result, err
	}
	pattern := regexp.MustCompile("^" + regexp.QuoteMeta(input.RawProjectPrefix) + `(\d+)_(\d+)$`)
	remapVarName := input.RemapVarName == nil || *input.RemapVarName
	enable := input.Enable == nil || *input.Enable
	for _, tag := range tags {
		matches := pattern.FindStringSubmatch(strings.TrimSpace(tag.RawName))
		if len(matches) != 3 {
			continue
		}
		projectNo, _ := strconv.Atoi(matches[1])
		varNo, _ := strconv.Atoi(matches[2])
		project, ok := projects[projectNo]
		if !ok {
			result.Skipped++
			result.Items = append(result.Items, BulkRemapKIOProjectsResultItem{
				VarID:     tag.VarID,
				VarIDText: strconv.FormatInt(tag.VarID, 10),
				RawName:   tag.RawName,
				ProjectNo: projectNo,
				Action:    "skipped",
				Reason:    "project number is outside configured range",
			})
			continue
		}
		newVarName := tag.VarName
		if remapVarName {
			newVarName = fmt.Sprintf("%s_%02d_%02d", input.VarNamePrefix, projectNo, varNo)
		}
		result.Matched++
		item := BulkRemapKIOProjectsResultItem{
			VarID:       tag.VarID,
			VarIDText:   strconv.FormatInt(tag.VarID, 10),
			RawName:     tag.RawName,
			OldVarName:  tag.VarName,
			NewVarName:  newVarName,
			ProjectNo:   projectNo,
			ProjectID:   project.ID,
			ProjectCode: project.ProjectCode,
			Action:      "updated",
		}
		if input.DryRun {
			item.Action = "dry_run"
			result.Items = append(result.Items, item)
			continue
		}
		updates := map[string]interface{}{
			"project_id":      &project.ID,
			"project_code":    project.ProjectCode,
			"var_group":       input.VarGroup,
			"var_name":        newVarName,
			"display_name":    tag.RawName,
			"display_name_en": fmt.Sprintf("%s %d Var %d", input.ProjectENPrefix, projectNo, varNo),
			"display_name_ja": fmt.Sprintf("%s%d 変数%d", input.ProjectJAPrefix, projectNo, varNo),
			"enabled":         enable,
		}
		updatedTag, err := s.repo.UpdateTag(tag.VarID, updates)
		if err != nil {
			return result, err
		}
		if enable {
			if _, err := s.repo.EnsureDefaultStorageRouteForTag(updatedTag); err != nil {
				return result, err
			}
		}
		result.Updated++
		result.Items = append(result.Items, item)
	}
	if !input.DryRun {
		if err := s.ReloadTags(); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (s *VariablesService) Create(input CreateVariableInput) (models.TagConfig, error) {
	sourceType := strings.ToLower(strings.TrimSpace(input.SourceType))
	if sourceType == "" {
		sourceType = models.TagSourceVirtual
	}
	if sourceType != models.TagSourceVirtual && sourceType != models.TagSourceManual {
		return models.TagConfig{}, fmt.Errorf("source_type must be virtual or manual")
	}
	dataType := strings.ToUpper(strings.TrimSpace(input.DataType))
	if !isSupportedVariableType(dataType) {
		return models.TagConfig{}, fmt.Errorf("data_type must be INT, FLOAT, BOOL, or STRING")
	}
	varName := strings.TrimSpace(input.VarName)
	if varName == "" {
		return models.TagConfig{}, fmt.Errorf("var_name is required")
	}
	ProjectCode := strings.TrimSpace(input.ProjectCode)
	if input.ProjectID != nil && ProjectCode == "" {
		Project, err := s.repo.GetProject(*input.ProjectID)
		if err != nil {
			return models.TagConfig{}, err
		}
		ProjectCode = Project.ProjectCode
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	tag := models.TagConfig{
		VarID:                  input.VarID,
		SourceType:             sourceType,
		GatewayID:              input.GatewayID,
		SourceTopic:            input.SourceTopic,
		SourcePath:             strings.TrimSpace(input.SourcePath),
		RawName:                firstNonEmpty(strings.TrimSpace(input.RawName), varName),
		ProjectID:              input.ProjectID,
		ProjectCode:            ProjectCode,
		VarGroup:               input.VarGroup,
		VarName:                varName,
		DisplayName:            firstNonEmpty(strings.TrimSpace(input.DisplayName), varName),
		DisplayNameEN:          input.DisplayNameEN,
		DisplayNameJA:          input.DisplayNameJA,
		JSONPath:               strings.TrimSpace(input.JSONPath),
		DataType:               dataType,
		Unit:                   input.Unit,
		DecimalPlaces:          input.DecimalPlaces,
		ScaleFactor:            input.ScaleFactor,
		OffsetVal:              input.OffsetVal,
		RWMode:                 input.RWMode,
		Writable:               input.Writable,
		WriteSourceID:          input.WriteSourceID,
		WritePath:              input.WritePath,
		WriteDataType:          input.WriteDataType,
		WriteMin:               input.WriteMin,
		WriteMax:               input.WriteMax,
		WriteEnum:              input.WriteEnum,
		SuspiciousValue:        input.SuspiciousValue,
		DebounceThreshold:      input.DebounceThreshold,
		DebounceMS:             input.DebounceMS,
		Deadband:               input.Deadband,
		DefaultLimitLL:         input.DefaultLimitLL,
		DefaultLimitL:          input.DefaultLimitL,
		DefaultLimitH:          input.DefaultLimitH,
		DefaultLimitHH:         input.DefaultLimitHH,
		DefaultLimitDeadband:   input.DefaultLimitDeadband,
		DefaultViolationHoldMS: input.DefaultViolationHoldMS,
		DefaultRecoverHoldMS:   input.DefaultRecoverHoldMS,
		Enabled:                enabled,
	}
	if input.WriteRequiresAudit == nil {
		tag.WriteRequiresAudit = true
	} else {
		tag.WriteRequiresAudit = *input.WriteRequiresAudit
	}
	if input.DefaultAlarmEnabled != nil {
		tag.DefaultAlarmEnabled = *input.DefaultAlarmEnabled
	}
	if tag.DecimalPlaces == 0 {
		tag.DecimalPlaces = 2
	}
	if tag.ScaleFactor == 0 {
		tag.ScaleFactor = 1
	}
	applyVariableDefaults(&tag)
	if sourceType == models.TagSourceVirtual {
		tag.GatewayID = 0
		tag.Discovered = false
		tag.Placeholder = true
		if tag.SourcePath == "" {
			ProjectPart := strings.TrimSpace(tag.ProjectCode)
			if ProjectPart == "" && tag.ProjectID != nil {
				ProjectPart = fmt.Sprintf("Project_%d", *tag.ProjectID)
			}
			if ProjectPart == "" {
				ProjectPart = "global"
			}
			tag.SourcePath = fmt.Sprintf("virtual.%s.%s", ProjectPart, varName)
		}
		if tag.JSONPath == "" {
			tag.JSONPath = tag.SourcePath
		}
	} else {
		tag.Discovered = false
		tag.Placeholder = false
		if tag.GatewayID <= 0 || tag.SourcePath == "" || tag.JSONPath == "" {
			return models.TagConfig{}, fmt.Errorf("manual variable requires gateway_id, source_path, and json_path")
		}
	}
	if err := validateVariableConfig(tag); err != nil {
		return models.TagConfig{}, err
	}
	if err := s.repo.CreateTag(&tag); err != nil {
		return models.TagConfig{}, err
	}
	if tag.ProjectID != nil {
		if err := s.repo.EnsureTagProjectGatewayEdge(tag.VarID, *tag.ProjectID); err != nil {
			return models.TagConfig{}, err
		}
	}
	if err := s.ReloadTags(); err != nil {
		return models.TagConfig{}, err
	}
	return tag, nil
}

func (s *VariablesService) Update(varID int64, updates map[string]interface{}, opts ...UpdateVariableOptions) (models.TagConfig, error) {
	if len(updates) > 0 {
		current, err := s.repo.GetTag(varID)
		if err != nil {
			return models.TagConfig{}, err
		}
		next := mergeTagUpdates(current, updates)
		applyVariableDefaults(&next)
		if err := validateVariableConfig(next); err != nil {
			return models.TagConfig{}, err
		}
	}
	tag, err := s.repo.UpdateTag(varID, updates)
	if err != nil {
		return models.TagConfig{}, err
	}
	if err := s.ReloadTags(); err != nil {
		return models.TagConfig{}, err
	}
	if len(opts) > 0 && opts[0].ApplyToRunning {
		if _, err := s.repo.UpdateRunningRunItemsVariableDefaults(varID, tag); err != nil {
			return models.TagConfig{}, err
		}
	}
	return tag, nil
}

func applyVariableDefaults(tag *models.TagConfig) {
	tag.SourceType = strings.ToLower(strings.TrimSpace(tag.SourceType))
	tag.DataType = strings.ToUpper(strings.TrimSpace(tag.DataType))
	tag.RWMode = strings.ToUpper(strings.TrimSpace(tag.RWMode))
	tag.WritePath = strings.TrimSpace(tag.WritePath)
	tag.WriteDataType = strings.ToUpper(strings.TrimSpace(tag.WriteDataType))
	tag.WriteEnum = strings.TrimSpace(tag.WriteEnum)
	if tag.RWMode == "" {
		tag.RWMode = models.RWModeRead
	}
	if tag.WriteDataType == "" && tag.Writable {
		tag.WriteDataType = strings.ToUpper(strings.TrimSpace(tag.DataType))
	}
}

func validateVariableConfig(tag models.TagConfig) error {
	if !isValidRWMode(tag.RWMode) {
		return fmt.Errorf("rw_mode must be R, W, or RW")
	}
	if tag.Writable {
		if tag.RWMode != models.RWModeWrite && tag.RWMode != models.RWModeReadWrite {
			return fmt.Errorf("writable variable requires rw_mode W or RW")
		}
		if tag.WritePath == "" {
			return fmt.Errorf("writable variable requires write_path")
		}
		if tag.WriteDataType == "" {
			return fmt.Errorf("writable variable requires write_data_type")
		}
		if !isSupportedVariableType(tag.WriteDataType) {
			return fmt.Errorf("write_data_type must be INT, FLOAT, BOOL, or STRING")
		}
	}
	if tag.WriteMin != nil && tag.WriteMax != nil && *tag.WriteMin > *tag.WriteMax {
		return fmt.Errorf("write_min must be less than or equal to write_max")
	}
	if tag.DebounceMS < 0 {
		return fmt.Errorf("debounce_ms must be greater than or equal to 0")
	}
	if tag.Deadband < 0 {
		return fmt.Errorf("deadband must be greater than or equal to 0")
	}
	if tag.DefaultLimitDeadband < 0 {
		return fmt.Errorf("default_limit_deadband must be greater than or equal to 0")
	}
	if tag.DefaultViolationHoldMS < 0 || tag.DefaultRecoverHoldMS < 0 {
		return fmt.Errorf("default alarm hold times must be greater than or equal to 0")
	}
	if err := validateLimitOrder("default", tag.DefaultLimitLL, tag.DefaultLimitL, tag.DefaultLimitH, tag.DefaultLimitHH); err != nil {
		return err
	}
	return nil
}

func isValidRWMode(mode string) bool {
	switch mode {
	case models.RWModeRead, models.RWModeWrite, models.RWModeReadWrite:
		return true
	default:
		return false
	}
}

func mergeTagUpdates(tag models.TagConfig, updates map[string]interface{}) models.TagConfig {
	for key, value := range updates {
		switch key {
		case "var_name":
			tag.VarName = value.(string)
		case "display_name":
			tag.DisplayName = value.(string)
		case "data_type":
			tag.DataType = value.(string)
		case "rw_mode":
			tag.RWMode = value.(string)
		case "writable":
			tag.Writable = value.(bool)
		case "write_source_id":
			tag.WriteSourceID = value.(int)
		case "write_path":
			tag.WritePath = value.(string)
		case "write_data_type":
			tag.WriteDataType = value.(string)
		case "write_min":
			tag.WriteMin = value.(*float64)
		case "write_max":
			tag.WriteMax = value.(*float64)
		case "write_enum":
			tag.WriteEnum = value.(string)
		case "write_requires_audit":
			tag.WriteRequiresAudit = value.(bool)
		case "suspicious_value":
			tag.SuspiciousValue = value.(*float64)
		case "debounce_threshold":
			tag.DebounceThreshold = value.(*float64)
		case "debounce_ms":
			tag.DebounceMS = value.(int)
		case "deadband":
			tag.Deadband = value.(float64)
		case "default_alarm_enabled":
			tag.DefaultAlarmEnabled = value.(bool)
		case "default_limit_ll":
			tag.DefaultLimitLL = value.(*float64)
		case "default_limit_l":
			tag.DefaultLimitL = value.(*float64)
		case "default_limit_h":
			tag.DefaultLimitH = value.(*float64)
		case "default_limit_hh":
			tag.DefaultLimitHH = value.(*float64)
		case "default_limit_deadband":
			tag.DefaultLimitDeadband = value.(float64)
		case "default_violation_hold_ms":
			tag.DefaultViolationHoldMS = value.(int)
		case "default_recover_hold_ms":
			tag.DefaultRecoverHoldMS = value.(int)
		case "enabled":
			tag.Enabled = value.(bool)
		}
	}
	return tag
}

func validateLimitOrder(prefix string, ll *float64, l *float64, h *float64, hh *float64) error {
	if ll != nil && l != nil && *ll > *l {
		return fmt.Errorf("%s_limit_ll must be less than or equal to %s_limit_l", prefix, prefix)
	}
	if l != nil && h != nil && *l > *h {
		return fmt.Errorf("%s_limit_l must be less than or equal to %s_limit_h", prefix, prefix)
	}
	if h != nil && hh != nil && *h > *hh {
		return fmt.Errorf("%s_limit_h must be less than or equal to %s_limit_hh", prefix, prefix)
	}
	return nil
}

func (s *VariablesService) Assign(varID int64, ProjectID *uint, ProjectCode string, group string, enabled bool) error {
	if err := s.repo.AssignTag(varID, ProjectID, ProjectCode, group, enabled); err != nil {
		return err
	}
	return s.ReloadTags()
}

func (s *VariablesService) Delete(varID int64) error {
	if err := s.repo.DeleteTag(varID); err != nil {
		return err
	}
	return s.ReloadTags()
}

func (s *VariablesService) ReloadTags() error {
	configs, err := s.repo.LoadTags(s.edgeInstanceID)
	if err != nil {
		return err
	}
	s.tags.Load(configs)
	return nil
}

func isSupportedVariableType(dataType string) bool {
	switch dataType {
	case "INT", "FLOAT", "BOOL", "STRING":
		return true
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func normalizeBulkRemapKIOInput(input BulkRemapKIOProjectsInput) BulkRemapKIOProjectsInput {
	if input.ProjectCount <= 0 {
		input.ProjectCount = 12
	}
	if input.ProjectCodePrefix == "" {
		input.ProjectCodePrefix = "AC"
	}
	if input.ProjectDisplayPrefix == "" {
		input.ProjectDisplayPrefix = "项目"
	}
	if input.ProjectENPrefix == "" {
		input.ProjectENPrefix = "Project"
	}
	if input.ProjectJAPrefix == "" {
		input.ProjectJAPrefix = "プロジェクト"
	}
	if input.RawProjectPrefix == "" {
		input.RawProjectPrefix = "台"
	}
	if input.VarGroup == "" {
		input.VarGroup = "KIO变量"
	}
	if input.VarNamePrefix == "" {
		input.VarNamePrefix = "kio"
	}
	return input
}
