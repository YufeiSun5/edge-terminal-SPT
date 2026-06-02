package database

import (
	"errors"
	"sort"
	"strconv"
	"strings"

	"spindle-edge/backend/internal/models"

	"gorm.io/gorm"
)

const defaultStationViewTemplateUID = "station-default"

type stationViewAssignmentCandidate struct {
	Assignment models.StationViewAssignment
	Template   models.StationViewTemplate
	Score      int
}

func (r *Repository) EnsureDefaultStationViewTemplate() error {
	var count int64
	if err := r.db.Model(&models.StationViewTemplate{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	template := models.StationViewTemplate{
		TemplateUID:   defaultStationViewTemplateUID,
		TemplateCode:  "STATION-DEFAULT",
		Name:          "Default station view",
		DisplayName:   "默认工位画面",
		DisplayNameEN: "Default station view",
		DisplayNameJA: "デフォルト工位画面",
		Version:       1,
		Status:        models.StationViewStatusPublished,
		OwnerScope:    "edge",
		LayoutJSON:    `{"auto_expand":true}`,
	}
	regions := []models.StationViewRegion{
		{
			TemplateUID: defaultStationViewTemplateUID,
			RegionKey:   "left",
			RegionType:  "metric_grid",
			LayoutJSON:  `{"columns":"auto","auto_expand":true}`,
			SortOrder:   10,
			Enabled:     true,
		},
		{
			TemplateUID: defaultStationViewTemplateUID,
			RegionKey:   "right",
			RegionType:  "inspection_table",
			LayoutJSON:  `{"auto_expand":true}`,
			SortOrder:   20,
			Enabled:     true,
		},
	}
	items := []models.StationViewItem{
		{
			TemplateUID: defaultStationViewTemplateUID,
			RegionKey:   "left",
			ItemUID:     "station-default-left-project-vars",
			ItemType:    "metric_card",
			BindingType: models.StationViewBindingVarGroup,
			BindingKey:  "",
			DisplayJSON: `{"source":"project_variables","auto_expand":true}`,
			SortOrder:   10,
			Visible:     true,
		},
		{
			TemplateUID: defaultStationViewTemplateUID,
			RegionKey:   "right",
			ItemUID:     "station-default-right-detection-items",
			ItemType:    "inspection_row",
			BindingType: models.StationViewBindingDetectionItems,
			BindingKey:  "",
			DisplayJSON: `{"source":"current_detection_run","auto_expand":true}`,
			SortOrder:   10,
			Visible:     true,
		},
		{
			TemplateUID: defaultStationViewTemplateUID,
			RegionKey:   "right",
			ItemUID:     "station-default-right-run-state",
			ItemType:    "run_state",
			BindingType: models.StationViewBindingRunState,
			BindingKey:  "",
			DisplayJSON: `{"source":"current_detection_run"}`,
			SortOrder:   20,
			Visible:     true,
		},
	}
	assignment := models.StationViewAssignment{
		TemplateUID: defaultStationViewTemplateUID,
		TargetType:  models.StationViewTargetGlobal,
		TargetKey:   "*",
		Priority:    0,
		Enabled:     true,
	}

	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&template).Error; err != nil {
			return err
		}
		if err := tx.Create(&regions).Error; err != nil {
			return err
		}
		if err := tx.Create(&items).Error; err != nil {
			return err
		}
		return tx.Create(&assignment).Error
	})
}

func (r *Repository) GetEffectiveStationView(projectID uint, edgeInstanceID string) (models.StationViewEffectiveResponse, error) {
	if projectID == 0 {
		return models.StationViewEffectiveResponse{}, gorm.ErrRecordNotFound
	}
	if err := r.EnsureDefaultStationViewTemplate(); err != nil {
		return models.StationViewEffectiveResponse{}, err
	}

	project, err := r.GetProject(projectID)
	if err != nil {
		return models.StationViewEffectiveResponse{}, err
	}
	if edgeInstanceID != "" && strings.TrimSpace(project.EdgeInstanceID) != "" && strings.TrimSpace(project.EdgeInstanceID) != edgeInstanceID {
		return models.StationViewEffectiveResponse{}, gorm.ErrRecordNotFound
	}

	template, err := r.resolveStationViewTemplate(project, strings.TrimSpace(edgeInstanceID))
	if err != nil {
		return models.StationViewEffectiveResponse{}, err
	}

	var regions []models.StationViewRegion
	if err := r.db.Where("template_uid = ? AND enabled = ?", template.TemplateUID, true).
		Order("sort_order ASC, id ASC").
		Find(&regions).Error; err != nil {
		return models.StationViewEffectiveResponse{}, err
	}
	var items []models.StationViewItem
	if err := r.db.Where("template_uid = ? AND visible = ?", template.TemplateUID, true).
		Order("region_key ASC, sort_order ASC, id ASC").
		Find(&items).Error; err != nil {
		return models.StationViewEffectiveResponse{}, err
	}

	tags, err := r.loadStationViewProjectTags(projectID)
	if err != nil {
		return models.StationViewEffectiveResponse{}, err
	}
	currentRun, hasCurrentRun, err := r.loadStationViewCurrentRun(projectID)
	if err != nil {
		return models.StationViewEffectiveResponse{}, err
	}

	warnings := []string{}
	seenVarIDs := map[int64]bool{}
	responseItems := make([]models.StationViewItemDTO, 0, len(items))
	httpCompanion := models.StationViewHTTPCompanion{}
	for _, item := range items {
		dto := models.StationViewItemDTO{
			ItemUID:     item.ItemUID,
			RegionKey:   item.RegionKey,
			ItemType:    item.ItemType,
			BindingType: item.BindingType,
			BindingKey:  item.BindingKey,
			BindingJSON: item.BindingJSON,
			DisplayJSON: item.DisplayJSON,
			SortOrder:   item.SortOrder,
		}
		bindings, itemWarnings := resolveStationViewItemBindings(item, tags, currentRun, hasCurrentRun)
		dto.ResolvedBindings = bindings
		warnings = append(warnings, itemWarnings...)
		for _, binding := range bindings {
			if binding.VarID > 0 {
				seenVarIDs[binding.VarID] = true
			}
		}
		if item.BindingType == models.StationViewBindingDetectionItems || item.BindingType == models.StationViewBindingRunState {
			httpCompanion.CurrentRunRequired = true
		}
		if item.BindingType == models.StationViewBindingAlarmSummary {
			httpCompanion.AlarmSummary = true
		}
		responseItems = append(responseItems, dto)
	}

	varIDs := make([]string, 0, len(seenVarIDs))
	for varID := range seenVarIDs {
		varIDs = append(varIDs, strconv.FormatInt(varID, 10))
	}
	sort.Slice(varIDs, func(i, j int) bool {
		left, _ := strconv.ParseInt(varIDs[i], 10, 64)
		right, _ := strconv.ParseInt(varIDs[j], 10, 64)
		return left < right
	})

	return models.StationViewEffectiveResponse{
		EdgeInstanceID: edgeInstanceID,
		Project: models.StationViewProjectRef{
			ID:            project.ID,
			ProjectCode:   project.ProjectCode,
			Name:          project.Name,
			DisplayName:   project.DisplayName,
			DisplayNameEN: project.DisplayNameEN,
			DisplayNameJA: project.DisplayNameJA,
			ModelName:     project.ModelName,
		},
		Template: models.StationViewTemplateRef{
			TemplateUID:   template.TemplateUID,
			TemplateCode:  template.TemplateCode,
			Name:          template.Name,
			DisplayName:   template.DisplayName,
			DisplayNameEN: template.DisplayNameEN,
			DisplayNameJA: template.DisplayNameJA,
			Version:       template.Version,
			Status:        template.Status,
			OwnerScope:    template.OwnerScope,
			LayoutJSON:    template.LayoutJSON,
		},
		Regions:        stationViewRegionDTOs(regions),
		Items:          responseItems,
		WSSubscription: models.StationViewWSSubscription{Topics: []string{"realtime.variables"}, ProjectID: projectID, VarIDs: varIDs},
		HTTPCompanion:  httpCompanion,
		Warnings:       warnings,
	}, nil
}

func (r *Repository) resolveStationViewTemplate(project models.Project, edgeInstanceID string) (models.StationViewTemplate, error) {
	var assignments []models.StationViewAssignment
	if err := r.db.Where("enabled = ?", true).Find(&assignments).Error; err != nil {
		return models.StationViewTemplate{}, err
	}
	candidates := make([]stationViewAssignmentCandidate, 0, len(assignments))
	for _, assignment := range assignments {
		score := stationViewAssignmentScore(assignment, project, edgeInstanceID)
		if score <= 0 {
			continue
		}
		var template models.StationViewTemplate
		err := r.db.Where("template_uid = ? AND status = ?", assignment.TemplateUID, models.StationViewStatusPublished).First(&template).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return models.StationViewTemplate{}, err
		}
		candidates = append(candidates, stationViewAssignmentCandidate{Assignment: assignment, Template: template, Score: score + assignment.Priority})
	}
	if len(candidates) == 0 {
		return models.StationViewTemplate{}, gorm.ErrRecordNotFound
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		if candidates[i].Template.Version != candidates[j].Template.Version {
			return candidates[i].Template.Version > candidates[j].Template.Version
		}
		return candidates[i].Assignment.ID < candidates[j].Assignment.ID
	})
	return candidates[0].Template, nil
}

func stationViewAssignmentScore(assignment models.StationViewAssignment, project models.Project, edgeInstanceID string) int {
	targetKey := strings.TrimSpace(assignment.TargetKey)
	switch assignment.TargetType {
	case models.StationViewTargetProject:
		if targetKey == project.ProjectCode || targetKey == strconv.FormatUint(uint64(project.ID), 10) {
			return 400
		}
	case models.StationViewTargetEdge:
		if edgeInstanceID != "" && targetKey == edgeInstanceID {
			return 300
		}
	case models.StationViewTargetModel:
		if project.ModelName != "" && targetKey == project.ModelName {
			return 200
		}
	case models.StationViewTargetGlobal:
		if targetKey == "" || targetKey == "*" {
			return 100
		}
	}
	return 0
}

func (r *Repository) loadStationViewProjectTags(projectID uint) ([]models.TagConfig, error) {
	var tags []models.TagConfig
	err := r.db.Where("project_id = ? AND enabled = ?", projectID, true).
		Order("var_group ASC, var_name ASC, var_id ASC").
		Find(&tags).Error
	return tags, err
}

func (r *Repository) loadStationViewCurrentRun(projectID uint) (models.DetectionTask, bool, error) {
	task, err := r.GetCurrentDetectionTaskForProject(projectID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.DetectionTask{}, false, nil
	}
	if err != nil {
		return models.DetectionTask{}, false, err
	}
	return task, true, nil
}

func resolveStationViewItemBindings(item models.StationViewItem, tags []models.TagConfig, currentRun models.DetectionTask, hasCurrentRun bool) ([]models.StationViewResolvedBinding, []string) {
	switch item.BindingType {
	case models.StationViewBindingVarGroup:
		return resolveStationViewVarGroup(item, tags), nil
	case models.StationViewBindingVarName:
		return resolveStationViewVarName(item, tags)
	case models.StationViewBindingDetectionItems:
		if !hasCurrentRun {
			return nil, []string{item.ItemUID + ": no current detection run"}
		}
		bindings := make([]models.StationViewResolvedBinding, 0, len(currentRun.StandardItems))
		for _, runItem := range currentRun.StandardItems {
			bindings = append(bindings, models.StationViewBindingFromRunItem(runItem))
		}
		sort.Slice(bindings, func(i, j int) bool {
			if bindings[i].SortOrder != bindings[j].SortOrder {
				return bindings[i].SortOrder < bindings[j].SortOrder
			}
			return bindings[i].VarID < bindings[j].VarID
		})
		return bindings, nil
	default:
		return nil, nil
	}
}

func resolveStationViewVarGroup(item models.StationViewItem, tags []models.TagConfig) []models.StationViewResolvedBinding {
	bindings := make([]models.StationViewResolvedBinding, 0, len(tags))
	for idx, tag := range tags {
		if item.BindingKey != "" && tag.VarGroup != item.BindingKey {
			continue
		}
		bindings = append(bindings, models.StationViewBindingFromTag("project_variable", tag, item.SortOrder+idx))
	}
	return bindings
}

func resolveStationViewVarName(item models.StationViewItem, tags []models.TagConfig) ([]models.StationViewResolvedBinding, []string) {
	bindings := []models.StationViewResolvedBinding{}
	for _, tag := range tags {
		if tag.VarName == item.BindingKey {
			bindings = append(bindings, models.StationViewBindingFromTag("project_variable", tag, item.SortOrder))
		}
	}
	if len(bindings) == 0 {
		return nil, []string{item.ItemUID + ": var_name not found in project"}
	}
	if len(bindings) > 1 {
		return bindings, []string{item.ItemUID + ": var_name matched multiple project variables"}
	}
	return bindings, nil
}

func stationViewRegionDTOs(regions []models.StationViewRegion) []models.StationViewRegionDTO {
	result := make([]models.StationViewRegionDTO, 0, len(regions))
	for _, region := range regions {
		result = append(result, models.StationViewRegionDTO{
			RegionKey:  region.RegionKey,
			RegionType: region.RegionType,
			LayoutJSON: region.LayoutJSON,
			SortOrder:  region.SortOrder,
		})
	}
	return result
}
