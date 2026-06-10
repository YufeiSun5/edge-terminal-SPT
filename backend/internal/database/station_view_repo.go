package database

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"spindle-edge/backend/internal/models"

	"gorm.io/gorm"
)

const defaultStationViewTemplateUID = "station-default"

var ErrStationViewTemplateConflict = errors.New("station view template assignment conflict")

type stationViewAssignmentCandidate struct {
	Assignment models.StationViewAssignment
	Template   models.StationViewTemplate
	Score      int
}

type StationViewTemplateFilter struct {
	Status     string
	OwnerScope string
	Keyword    string
}

func (r *Repository) EnsureDefaultStationViewTemplate() error {
	var count int64
	if err := r.db.Model(&models.StationViewTemplate{}).Where("template_uid = ?", defaultStationViewTemplateUID).Count(&count).Error; err != nil {
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
			RegionKey:   models.StationViewLayoutAreaCardPool,
			LayoutArea:  models.StationViewLayoutAreaCardPool,
			RegionType:  "metric_grid",
			LayoutJSON:  `{"columns":"auto","auto_expand":true}`,
			SortOrder:   10,
			Enabled:     true,
		},
		{
			TemplateUID: defaultStationViewTemplateUID,
			RegionKey:   models.StationViewLayoutAreaListLayout,
			LayoutArea:  models.StationViewLayoutAreaListLayout,
			RegionType:  "inspection_table",
			LayoutJSON:  `{"auto_expand":true}`,
			SortOrder:   20,
			Enabled:     true,
		},
	}
	items := []models.StationViewItem{
		{
			TemplateUID: defaultStationViewTemplateUID,
			RegionKey:   models.StationViewLayoutAreaCardPool,
			LayoutArea:  models.StationViewLayoutAreaCardPool,
			ItemUID:     "station-default-card-pool-project-vars",
			ItemType:    "metric_card",
			BindingType: models.StationViewBindingVarGroup,
			BindingKey:  "",
			DisplayJSON: `{"source":"project_variables","auto_expand":true}`,
			SortOrder:   10,
			Visible:     true,
		},
		{
			TemplateUID: defaultStationViewTemplateUID,
			RegionKey:   models.StationViewLayoutAreaListLayout,
			LayoutArea:  models.StationViewLayoutAreaListLayout,
			ItemUID:     "station-default-list-layout-detection-items",
			ItemType:    "inspection_row",
			BindingType: models.StationViewBindingDetectionItems,
			BindingKey:  "",
			DisplayJSON: `{"source":"current_detection_run","auto_expand":true}`,
			SortOrder:   10,
			Visible:     true,
		},
		{
			TemplateUID: defaultStationViewTemplateUID,
			RegionKey:   models.StationViewLayoutAreaListLayout,
			LayoutArea:  models.StationViewLayoutAreaListLayout,
			ItemUID:     "station-default-list-layout-run-state",
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
		var assignmentCount int64
		if err := tx.Model(&models.StationViewAssignment{}).
			Where("target_type = ? AND target_key = ?", assignment.TargetType, assignment.TargetKey).
			Count(&assignmentCount).Error; err != nil {
			return err
		}
		if assignmentCount > 0 {
			return nil
		}
		return tx.Create(&assignment).Error
	})
}

func (r *Repository) ListStationViewTemplates(filter StationViewTemplateFilter) ([]models.StationViewTemplateListItem, error) {
	if err := r.EnsureDefaultStationViewTemplate(); err != nil {
		return nil, err
	}
	query := r.db.Model(&models.StationViewTemplate{})
	if status := strings.TrimSpace(filter.Status); status != "" {
		query = query.Where("status = ?", status)
	}
	if ownerScope := strings.TrimSpace(filter.OwnerScope); ownerScope != "" {
		query = query.Where("owner_scope = ?", ownerScope)
	}
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("template_code LIKE ? OR name LIKE ? OR display_name LIKE ?", like, like, like)
	}
	var templates []models.StationViewTemplate
	if err := query.Order("updated_at DESC, id DESC").Find(&templates).Error; err != nil {
		return nil, err
	}
	var assignments []models.StationViewAssignment
	if err := r.db.Order("target_type ASC, target_key ASC, priority DESC, id ASC").Find(&assignments).Error; err != nil {
		return nil, err
	}
	assignmentsByTemplate := make(map[string][]models.StationViewAssignmentDTO)
	for _, assignment := range assignments {
		assignmentsByTemplate[assignment.TemplateUID] = append(assignmentsByTemplate[assignment.TemplateUID], stationViewAssignmentDTO(assignment))
	}
	items := make([]models.StationViewTemplateListItem, 0, len(templates))
	for _, template := range templates {
		items = append(items, stationViewTemplateListItem(template, assignmentsByTemplate[template.TemplateUID]))
	}
	return items, nil
}

func (r *Repository) StationViewDiagnostics() (models.StationViewDiagnostics, error) {
	if err := r.EnsureDefaultStationViewTemplate(); err != nil {
		return models.StationViewDiagnostics{}, err
	}
	diagnostics := models.StationViewDiagnostics{Status: "ok"}
	if err := r.db.Model(&models.StationViewTemplate{}).Count(&diagnostics.TemplateCount).Error; err != nil {
		return models.StationViewDiagnostics{}, err
	}
	if err := r.db.Model(&models.StationViewTemplate{}).Where("status = ?", models.StationViewStatusPublished).Count(&diagnostics.PublishedTemplates).Error; err != nil {
		return models.StationViewDiagnostics{}, err
	}
	if err := r.db.Model(&models.StationViewRegion{}).Count(&diagnostics.RegionCount).Error; err != nil {
		return models.StationViewDiagnostics{}, err
	}
	if err := r.db.Model(&models.StationViewItem{}).Count(&diagnostics.ItemCount).Error; err != nil {
		return models.StationViewDiagnostics{}, err
	}
	if err := r.db.Model(&models.StationViewAssignment{}).Count(&diagnostics.AssignmentCount).Error; err != nil {
		return models.StationViewDiagnostics{}, err
	}
	if err := r.db.Model(&models.StationViewAssignment{}).Where("enabled = ?", true).Count(&diagnostics.EnabledAssignments).Error; err != nil {
		return models.StationViewDiagnostics{}, err
	}
	var defaultCount int64
	if err := r.db.Model(&models.StationViewTemplate{}).Where("template_uid = ?", defaultStationViewTemplateUID).Count(&defaultCount).Error; err != nil {
		return models.StationViewDiagnostics{}, err
	}
	diagnostics.DefaultTemplateReady = defaultCount > 0
	if diagnostics.PublishedTemplates == 0 {
		diagnostics.Status = "degraded"
		diagnostics.Warnings = append(diagnostics.Warnings, "no published station view template")
	}
	if diagnostics.EnabledAssignments == 0 {
		diagnostics.Status = "degraded"
		diagnostics.Warnings = append(diagnostics.Warnings, "no enabled station view assignment")
	}
	return diagnostics, nil
}

func (r *Repository) ReloadStationView(projectID uint, edgeInstanceID string) (models.StationViewReloadResponse, error) {
	diagnostics, err := r.StationViewDiagnostics()
	if err != nil {
		return models.StationViewReloadResponse{}, err
	}
	response := models.StationViewReloadResponse{
		OK:             diagnostics.Status == "ok",
		EdgeInstanceID: strings.TrimSpace(edgeInstanceID),
		ReloadMode:     "validate_and_seed",
		Diagnostics:    diagnostics,
	}
	if projectID > 0 {
		effective, err := r.GetEffectiveStationView(projectID, edgeInstanceID)
		if err != nil {
			return models.StationViewReloadResponse{}, err
		}
		response.Effective = &effective
	}
	return response, nil
}

func (r *Repository) UpdateStationViewTemplate(id uint, updates map[string]interface{}) (models.StationViewTemplate, error) {
	if id == 0 {
		return models.StationViewTemplate{}, gorm.ErrRecordNotFound
	}
	updates = stationViewUpdateWithTime(updates)
	if err := r.db.Model(&models.StationViewTemplate{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return models.StationViewTemplate{}, err
	}
	var template models.StationViewTemplate
	if err := r.db.First(&template, "id = ?", id).Error; err != nil {
		return models.StationViewTemplate{}, err
	}
	return template, nil
}

func (r *Repository) UpdateStationViewAssignment(id uint, updates map[string]interface{}) (models.StationViewAssignment, error) {
	if id == 0 {
		return models.StationViewAssignment{}, gorm.ErrRecordNotFound
	}
	updates = stationViewUpdateWithTime(updates)
	if err := r.db.Model(&models.StationViewAssignment{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return models.StationViewAssignment{}, err
	}
	var assignment models.StationViewAssignment
	if err := r.db.First(&assignment, "id = ?", id).Error; err != nil {
		return models.StationViewAssignment{}, err
	}
	return assignment, nil
}

func (r *Repository) ListStationViewItems(templateUID string) ([]models.StationViewItemDTO, error) {
	templateUID = strings.TrimSpace(templateUID)
	if templateUID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	if err := r.EnsureDefaultStationViewTemplate(); err != nil {
		return nil, err
	}
	var template models.StationViewTemplate
	if err := r.db.First(&template, "template_uid = ?", templateUID).Error; err != nil {
		return nil, err
	}
	var items []models.StationViewItem
	if err := r.db.Where("template_uid = ?", templateUID).
		Order("layout_area ASC, sort_order ASC, id ASC").
		Find(&items).Error; err != nil {
		return nil, err
	}
	return stationViewItemDTOs(items), nil
}

func (r *Repository) ReplaceStationViewItems(templateUID string, items []models.StationViewItemDTO) ([]models.StationViewItemDTO, error) {
	templateUID = strings.TrimSpace(templateUID)
	if templateUID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	if err := r.EnsureDefaultStationViewTemplate(); err != nil {
		return nil, err
	}
	now := time.Now()
	cleaned := make([]models.StationViewItem, 0, len(items))
	hiddenItemUIDs := make([]string, 0)
	seenItemUIDs := make(map[string]bool, len(items))
	areas := map[string]bool{}
	for idx, item := range items {
		cleanedItem, err := stationViewItemFromDTO(templateUID, item, idx, now)
		if err != nil {
			return nil, err
		}
		if seenItemUIDs[cleanedItem.ItemUID] {
			return nil, errors.New("duplicate station view item_uid")
		}
		seenItemUIDs[cleanedItem.ItemUID] = true
		if !cleanedItem.Visible {
			hiddenItemUIDs = append(hiddenItemUIDs, cleanedItem.ItemUID)
		}
		areas[cleanedItem.LayoutArea] = true
		cleaned = append(cleaned, cleanedItem)
	}
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var template models.StationViewTemplate
		if err := tx.First(&template, "template_uid = ?", templateUID).Error; err != nil {
			return err
		}
		for area := range areas {
			if err := ensureStationViewAreaRegion(tx, templateUID, area, now); err != nil {
				return err
			}
		}
		if err := tx.Delete(&models.StationViewItem{}, "template_uid = ?", templateUID).Error; err != nil {
			return err
		}
		if len(cleaned) == 0 {
			return nil
		}
		if err := tx.Select("*").Create(&cleaned).Error; err != nil {
			return err
		}
		for _, itemUID := range hiddenItemUIDs {
			if err := tx.Model(&models.StationViewItem{}).
				Where("template_uid = ? AND item_uid = ?", templateUID, itemUID).
				Update("visible", false).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return r.ListStationViewItems(templateUID)
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
		Order("layout_area ASC, sort_order ASC, id ASC").
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
			LayoutArea:  stationViewItemLayoutArea(item),
			ItemType:    item.ItemType,
			BindingType: item.BindingType,
			BindingKey:  item.BindingKey,
			BindingJSON: item.BindingJSON,
			DisplayJSON: item.DisplayJSON,
			SortOrder:   item.SortOrder,
			Visible:     item.Visible,
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

func stationViewTemplateListItem(template models.StationViewTemplate, assignments []models.StationViewAssignmentDTO) models.StationViewTemplateListItem {
	return models.StationViewTemplateListItem{
		ID:            template.ID,
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
		Assignments:   assignments,
		CreatedAt:     template.CreatedAt,
		UpdatedAt:     template.UpdatedAt,
	}
}

func stationViewAssignmentDTO(assignment models.StationViewAssignment) models.StationViewAssignmentDTO {
	return models.StationViewAssignmentDTO(assignment)
}

func stationViewUpdateWithTime(updates map[string]interface{}) map[string]interface{} {
	if updates == nil {
		updates = map[string]interface{}{}
	}
	updates["updated_at"] = time.Now()
	return updates
}

func stationViewItemDTOs(items []models.StationViewItem) []models.StationViewItemDTO {
	result := make([]models.StationViewItemDTO, 0, len(items))
	for _, item := range items {
		result = append(result, models.StationViewItemDTO{
			ItemUID:     item.ItemUID,
			LayoutArea:  stationViewItemLayoutArea(item),
			ItemType:    item.ItemType,
			BindingType: item.BindingType,
			BindingKey:  item.BindingKey,
			BindingJSON: item.BindingJSON,
			DisplayJSON: item.DisplayJSON,
			SortOrder:   item.SortOrder,
			Visible:     item.Visible,
		})
	}
	return result
}

func stationViewItemFromDTO(templateUID string, item models.StationViewItemDTO, index int, now time.Time) (models.StationViewItem, error) {
	layoutArea := strings.TrimSpace(item.LayoutArea)
	if err := validateStationViewLayoutArea(layoutArea); err != nil {
		return models.StationViewItem{}, err
	}
	regionKey := layoutArea
	itemUID := strings.TrimSpace(item.ItemUID)
	if itemUID == "" {
		return models.StationViewItem{}, errors.New("station view item_uid is required")
	}
	itemType := strings.TrimSpace(item.ItemType)
	if itemType == "" {
		return models.StationViewItem{}, errors.New("station view item_type is required")
	}
	bindingType := strings.TrimSpace(item.BindingType)
	if err := validateStationViewBindingType(bindingType); err != nil {
		return models.StationViewItem{}, err
	}
	sortOrder := item.SortOrder
	if sortOrder == 0 {
		sortOrder = (index + 1) * 10
	}
	return models.StationViewItem{
		TemplateUID: templateUID,
		RegionKey:   regionKey,
		LayoutArea:  layoutArea,
		ItemUID:     itemUID,
		ItemType:    itemType,
		BindingType: bindingType,
		BindingKey:  strings.TrimSpace(item.BindingKey),
		BindingJSON: strings.TrimSpace(item.BindingJSON),
		DisplayJSON: strings.TrimSpace(item.DisplayJSON),
		SortOrder:   sortOrder,
		Visible:     item.Visible,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func validateStationViewBindingType(bindingType string) error {
	switch bindingType {
	case models.StationViewBindingVarName,
		models.StationViewBindingVarGroup,
		models.StationViewBindingDetectionItems,
		models.StationViewBindingAlarmSummary,
		models.StationViewBindingRunState,
		models.StationViewBindingManual:
		return nil
	default:
		return errors.New("invalid station view binding_type")
	}
}

func validateStationViewLayoutArea(layoutArea string) error {
	switch layoutArea {
	case models.StationViewLayoutAreaCardPool, models.StationViewLayoutAreaListLayout:
		return nil
	default:
		return errors.New("invalid station view layout_area")
	}
}

func stationViewItemLayoutArea(item models.StationViewItem) string {
	if layoutArea := strings.TrimSpace(item.LayoutArea); layoutArea != "" {
		return layoutArea
	}
	switch item.RegionKey {
	case "left":
		return models.StationViewLayoutAreaCardPool
	case "right":
		return models.StationViewLayoutAreaListLayout
	default:
		return strings.TrimSpace(item.RegionKey)
	}
}

func stationViewRegionLayoutArea(region models.StationViewRegion) string {
	if layoutArea := strings.TrimSpace(region.LayoutArea); layoutArea != "" {
		return layoutArea
	}
	switch region.RegionKey {
	case "left":
		return models.StationViewLayoutAreaCardPool
	case "right":
		return models.StationViewLayoutAreaListLayout
	default:
		return strings.TrimSpace(region.RegionKey)
	}
}

func ensureStationViewAreaRegion(tx *gorm.DB, templateUID string, layoutArea string, now time.Time) error {
	if err := validateStationViewLayoutArea(layoutArea); err != nil {
		return err
	}
	regionKey := layoutArea
	var existing models.StationViewRegion
	err := tx.First(&existing, "template_uid = ? AND layout_area = ?", templateUID, layoutArea).Error
	if err == nil {
		updates := map[string]interface{}{"region_key": regionKey, "layout_area": layoutArea, "enabled": true, "updated_at": now}
		if existing.RegionType == "" {
			if layoutArea == models.StationViewLayoutAreaCardPool {
				updates["region_type"] = "metric_grid"
			} else {
				updates["region_type"] = "inspection_table"
			}
		}
		return tx.Model(&models.StationViewRegion{}).Where("id = ?", existing.ID).Updates(updates).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	regionType := "inspection_table"
	sortOrder := 20
	if layoutArea == models.StationViewLayoutAreaCardPool {
		regionType = "metric_grid"
		sortOrder = 10
	}
	return tx.Create(&models.StationViewRegion{
		TemplateUID: templateUID,
		RegionKey:   regionKey,
		LayoutArea:  layoutArea,
		RegionType:  regionType,
		SortOrder:   sortOrder,
		Enabled:     true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}).Error
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
	if len(candidates) > 1 && candidates[0].Score == candidates[1].Score && candidates[0].Template.Version == candidates[1].Template.Version {
		return models.StationViewTemplate{}, ErrStationViewTemplateConflict
	}
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
			LayoutArea: stationViewRegionLayoutArea(region),
			LayoutType: region.RegionType,
			LayoutJSON: region.LayoutJSON,
			SortOrder:  region.SortOrder,
		})
	}
	return result
}
