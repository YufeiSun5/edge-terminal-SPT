package database

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"spindle-edge/backend/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestRepositoryAuthMethods(t *testing.T) {
	db := newRepositoryTestDB(t)
	repo := NewRepository(db)
	count, err := repo.CountUsers()
	if err != nil || count != 0 {
		t.Fatalf("unexpected user count=%d err=%v", count, err)
	}
	user := &models.SysUser{Username: "admin", PasswordHash: "hash", Role: "admin", Enabled: true}
	if err := repo.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	if user.PermissionsVersion != 1 || user.CreatedAt.IsZero() {
		t.Fatalf("unexpected user defaults: %+v", user)
	}
	if got, err := repo.FindUserByUsername("admin"); err != nil || got.ID != user.ID {
		t.Fatalf("find by username got=%+v err=%v", got, err)
	}
	if got, err := repo.FindUserByID(user.ID); err != nil || got.Username != "admin" {
		t.Fatalf("find by id got=%+v err=%v", got, err)
	}
	now := time.Now()
	if err := repo.UpdateUserLastLogin(user.ID, now); err != nil {
		t.Fatal(err)
	}
	if users, err := repo.ListUsers(); err != nil || len(users) != 1 {
		t.Fatalf("list users len=%d err=%v", len(users), err)
	}
	updatedUser, err := repo.UpdateUser(user.ID, map[string]interface{}{"role": "operator", "enabled": false})
	if err != nil {
		t.Fatal(err)
	}
	if updatedUser.Role != "operator" || updatedUser.Enabled || updatedUser.PermissionsVersion != 2 {
		t.Fatalf("unexpected updated user: %+v", updatedUser)
	}
	updatedUser, err = repo.UpdateUser(user.ID, nil)
	if err != nil || updatedUser.ID != user.ID {
		t.Fatalf("empty update user got=%+v err=%v", updatedUser, err)
	}
	if _, err := repo.UpdateUser(user.ID, map[string]interface{}{"role": "admin", "enabled": true}); err != nil {
		t.Fatal(err)
	}
	fetchedUser, err := repo.FindUserByID(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	user.PermissionsVersion = fetchedUser.PermissionsVersion

	client := models.SysServiceClient{ClientID: "main", SecretHash: "hash-1", Scopes: "service_sso_verify", Enabled: true}
	if err := repo.UpsertServiceClient(client); err != nil {
		t.Fatal(err)
	}
	client.SecretHash = "hash-2"
	client.Enabled = false
	if err := repo.UpsertServiceClient(client); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.FindServiceClientBySecretHash("hash-2"); err != nil || got.ClientID != "main" || got.Enabled {
		t.Fatalf("unexpected client got=%+v err=%v", got, err)
	}

	ticket := &models.SysSSOTicket{
		TicketHash:         "ticket",
		UserID:             user.ID,
		Role:               "admin",
		PermissionsVersion: user.PermissionsVersion,
		EdgeInstanceID:     "edge-1",
		ExpiresAt:          now.Add(time.Minute),
	}
	if err := repo.CreateSSOTicket(ticket); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.ConsumeSSOTicket("ticket", "edge-1", now); err != nil || got.ID != user.ID {
		t.Fatalf("consume ticket got=%+v err=%v", got, err)
	}
	if _, err := repo.ConsumeSSOTicket("ticket", "edge-1", now); !errors.Is(err, ErrSSOTicketUsed) {
		t.Fatalf("expected used ticket error, got %v", err)
	}
	expired := &models.SysSSOTicket{
		TicketHash:         "expired",
		UserID:             user.ID,
		Role:               "admin",
		PermissionsVersion: user.PermissionsVersion,
		EdgeInstanceID:     "edge-1",
		ExpiresAt:          now.Add(-time.Second),
	}
	if err := repo.CreateSSOTicket(expired); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ConsumeSSOTicket("expired", "edge-1", now); !errors.Is(err, ErrSSOTicketExpired) {
		t.Fatalf("expected expired ticket error, got %v", err)
	}
	if _, err := repo.ConsumeSSOTicket("missing", "edge-1", now); !errors.Is(err, ErrSSOTicketInvalid) {
		t.Fatalf("expected invalid ticket error, got %v", err)
	}
	if err := repo.CreateAuditLog(&models.SysAuditLog{ActorType: "user", ActorID: "1", Action: "test", Result: "success"}); err != nil {
		t.Fatal(err)
	}
	logs, total, err := repo.ListAuditLogs(AuditLogListFilter{ActorType: "user", ActorID: "1", Action: "test", Result: "success", From: &now, Limit: 500, Offset: -1})
	if err != nil || total != 1 || len(logs) != 1 {
		t.Fatalf("audit logs len=%d total=%d err=%v", len(logs), total, err)
	}
	tmp := &models.SysUser{Username: "delete-me", PasswordHash: "hash", Role: "operator", Enabled: true}
	if err := repo.CreateUser(tmp); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteUser(tmp.ID); err != nil {
		t.Fatal(err)
	}
}

func TestRepositoryNotificationMethods(t *testing.T) {
	db := newRepositoryTestDB(t)
	repo := NewRepository(db)
	admin := &models.SysUser{Username: "admin", PasswordHash: "hash", Role: "admin", Enabled: true}
	operator := &models.SysUser{Username: "operator", PasswordHash: "hash", Role: "operator", Enabled: true}
	disabled := &models.SysUser{Username: "disabled", PasswordHash: "hash", Role: "operator", Enabled: false}
	for _, user := range []*models.SysUser{admin, operator, disabled} {
		if err := repo.CreateUser(user); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.UpdateUser(disabled.ID, map[string]interface{}{"enabled": false}); err != nil {
		t.Fatal(err)
	}

	occurredAt := time.Date(2026, 5, 30, 18, 30, 0, 0, time.UTC)
	notification := &models.RuntimeNotification{
		ID:          "event-1",
		Type:        models.NotificationDetectionResultNG,
		Level:       models.NotificationLevelWarning,
		ProjectID:   7,
		ProjectCode: "AC-07",
		TaskID:      55,
		TestNo:      "T-55",
		Message:     "NG",
		Payload:     map[string]any{"result_status": models.DetectionSummaryStatusNG},
		OccurredAt:  occurredAt,
	}
	item, err := repo.CreateRuntimeNotification(notification)
	if err != nil {
		t.Fatal(err)
	}
	if item.ID == 0 || item.TargetType != models.NotificationTargetProject || item.TargetID != "7" || item.Payload == "" {
		t.Fatalf("unexpected persisted notification: %+v", item)
	}

	unread, err := repo.CountUnreadNotifications(admin.ID)
	if err != nil || unread != 1 {
		t.Fatalf("unread=%d err=%v", unread, err)
	}
	disabledUnread, err := repo.CountUnreadNotifications(disabled.ID)
	if err != nil || disabledUnread != 0 {
		t.Fatalf("disabled unread=%d err=%v", disabledUnread, err)
	}
	projectID := uint(7)
	items, total, err := repo.ListUserNotifications(NotificationListFilter{UserID: admin.ID, Unread: boolTestPtr(true), Type: models.NotificationDetectionResultNG, Level: models.NotificationLevelWarning, ProjectID: &projectID, Limit: 10})
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("items=%+v total=%d err=%v", items, total, err)
	}
	if items[0].Payload == "" || items[0].ReadAt != nil || items[0].ProjectCode != "AC-07" {
		t.Fatalf("unexpected notification item: %+v", items[0])
	}
	if err := repo.MarkNotificationRead(admin.ID, item.ID); err != nil {
		t.Fatal(err)
	}
	if unread, err := repo.CountUnreadNotifications(admin.ID); err != nil || unread != 0 {
		t.Fatalf("unread after mark=%d err=%v", unread, err)
	}
	if err := repo.MarkNotificationRead(admin.ID, item.ID+999); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected record not found, got %v", err)
	}
	updated, err := repo.MarkAllNotificationsRead(operator.ID)
	if err != nil || updated != 1 {
		t.Fatalf("mark all updated=%d err=%v", updated, err)
	}

	if _, err := repo.CreateRuntimeNotification(notification); err != nil {
		t.Fatal(err)
	}
	var totalNotifications int64
	if err := db.Model(&models.SysNotification{}).Count(&totalNotifications).Error; err != nil {
		t.Fatal(err)
	}
	if totalNotifications != 1 {
		t.Fatalf("expected duplicate event_uid to be ignored, got %d", totalNotifications)
	}

	expiredOccurredAt := time.Now().AddDate(0, 0, -models.AlarmNotificationRetentionDays-1)
	expiredAlarm, err := repo.CreateRuntimeNotification(&models.RuntimeNotification{
		ID:          "event-expired-alarm",
		Type:        models.NotificationAlarmLimitEnter,
		Level:       models.NotificationLevelWarning,
		ProjectID:   7,
		ProjectCode: "AC-07",
		Message:     "expired alarm",
		OccurredAt:  expiredOccurredAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if expiredAlarm.ExpiresAt == nil || !expiredAlarm.ExpiresAt.Equal(expiredOccurredAt.AddDate(0, 0, models.AlarmNotificationRetentionDays)) {
		t.Fatalf("unexpected expired alarm expires_at: %+v", expiredAlarm)
	}
	if unread, err := repo.CountUnreadNotifications(admin.ID); err != nil || unread != 0 {
		t.Fatalf("expired alarm should not count as unread unread=%d err=%v", unread, err)
	}
	alarmItems, alarmTotal, err := repo.ListUserNotifications(NotificationListFilter{UserID: admin.ID, Type: models.NotificationAlarmLimitEnter, Limit: 10})
	if err != nil || alarmTotal != 0 || len(alarmItems) != 0 {
		t.Fatalf("expired alarm should be hidden items=%+v total=%d err=%v", alarmItems, alarmTotal, err)
	}
	if err := repo.MarkNotificationRead(admin.ID, expiredAlarm.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expired alarm should not be readable from notification center, got %v", err)
	}
	oldNonAlarm, err := repo.CreateRuntimeNotification(&models.RuntimeNotification{
		ID:         "event-old-non-alarm",
		Type:       models.NotificationDetectionFeatures,
		Level:      models.NotificationLevelInfo,
		TargetType: models.NotificationTargetUser,
		TargetID:   strconv.FormatUint(uint64(admin.ID), 10),
		Message:    "old non-alarm",
		OccurredAt: expiredOccurredAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if oldNonAlarm.ExpiresAt != nil {
		t.Fatalf("non-alarm notification should not expire: %+v", oldNonAlarm)
	}
	if unread, err := repo.CountUnreadNotifications(admin.ID); err != nil || unread != 1 {
		t.Fatalf("old non-alarm should remain visible unread=%d err=%v", unread, err)
	}
	if err := repo.MarkNotificationRead(admin.ID, oldNonAlarm.ID); err != nil {
		t.Fatal(err)
	}

	filterStart := time.Now().Add(-time.Minute)
	filteredMatch, err := repo.CreateRuntimeNotification(&models.RuntimeNotification{
		ID:         "event-filter-match",
		Type:       models.NotificationDetectionRunPaused,
		Level:      models.NotificationLevelInfo,
		TargetType: models.NotificationTargetUser,
		TargetID:   strconv.FormatUint(uint64(admin.ID), 10),
		ProjectID:  7,
		Message:    "match target",
		OccurredAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateRuntimeNotification(&models.RuntimeNotification{
		ID:         "event-filter-project-miss",
		Type:       models.NotificationDetectionRunPaused,
		Level:      models.NotificationLevelInfo,
		TargetType: models.NotificationTargetUser,
		TargetID:   strconv.FormatUint(uint64(admin.ID), 10),
		ProjectID:  8,
		Message:    "match target",
		OccurredAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateRuntimeNotification(&models.RuntimeNotification{
		ID:         "event-filter-keyword-miss",
		Type:       models.NotificationDetectionRunPaused,
		Level:      models.NotificationLevelInfo,
		TargetType: models.NotificationTargetUser,
		TargetID:   strconv.FormatUint(uint64(admin.ID), 10),
		ProjectID:  7,
		Message:    "other target",
		OccurredAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if unread, err := repo.CountUnreadNotifications(admin.ID); err != nil || unread != 3 {
		t.Fatalf("filtered setup unread=%d err=%v", unread, err)
	}
	if unread, err := repo.CountUnreadNotificationsWithFilter(NotificationListFilter{
		UserID:    admin.ID,
		Type:      models.NotificationDetectionRunPaused,
		ProjectID: &projectID,
		From:      &filterStart,
		Keyword:   "match",
	}); err != nil || unread != 1 {
		t.Fatalf("filtered unread count=%d err=%v", unread, err)
	}
	updated, err = repo.MarkUserNotificationsRead(NotificationListFilter{
		UserID:    admin.ID,
		Type:      models.NotificationDetectionRunPaused,
		ProjectID: &projectID,
		From:      &filterStart,
		Keyword:   "match",
	})
	if err != nil || updated != 1 {
		t.Fatalf("filtered mark read updated=%d err=%v", updated, err)
	}
	var recipient models.SysNotificationRecipient
	if err := db.Where("notification_id = ? AND user_id = ?", filteredMatch.ID, admin.ID).First(&recipient).Error; err != nil || recipient.ReadAt == nil {
		t.Fatalf("filtered match should be read recipient=%+v err=%v", recipient, err)
	}
	if unread, err := repo.CountUnreadNotifications(admin.ID); err != nil || unread != 2 {
		t.Fatalf("filtered mark should leave two unread notifications unread=%d err=%v", unread, err)
	}
	if updated, err := repo.MarkUserNotificationsRead(NotificationListFilter{UserID: admin.ID, Unread: boolTestPtr(false)}); err != nil || updated != 0 {
		t.Fatalf("read-all with unread=false should be no-op updated=%d err=%v", updated, err)
	}
	if _, err := repo.MarkAllNotificationsRead(admin.ID); err != nil {
		t.Fatal(err)
	}

	roleNotification := &models.RuntimeNotification{
		ID:         "event-role",
		Type:       models.NotificationDetectionRunPaused,
		Level:      models.NotificationLevelInfo,
		TargetType: models.NotificationTargetRole,
		TargetID:   "operator",
		Message:    "role target",
		OccurredAt: occurredAt.Add(time.Minute),
	}
	roleItem, err := repo.CreateRuntimeNotification(roleNotification)
	if err != nil {
		t.Fatal(err)
	}
	if roleItem.TargetType != models.NotificationTargetRole || roleItem.TargetID != "operator" {
		t.Fatalf("unexpected role notification target: %+v", roleItem)
	}
	if unread, err := repo.CountUnreadNotifications(admin.ID); err != nil || unread != 0 {
		t.Fatalf("admin should not receive role notification unread=%d err=%v", unread, err)
	}
	if unread, err := repo.CountUnreadNotifications(operator.ID); err != nil || unread != 1 {
		t.Fatalf("operator should receive role notification unread=%d err=%v", unread, err)
	}

	userNotification := &models.RuntimeNotification{
		ID:         "event-user",
		Type:       models.NotificationDetectionRunResumed,
		Level:      models.NotificationLevelInfo,
		TargetType: models.NotificationTargetUser,
		TargetID:   strconv.FormatUint(uint64(admin.ID), 10),
		Message:    "user target",
		OccurredAt: occurredAt.Add(2 * time.Minute),
	}
	if _, err := repo.CreateRuntimeNotification(userNotification); err != nil {
		t.Fatal(err)
	}
	if unread, err := repo.CountUnreadNotifications(admin.ID); err != nil || unread != 1 {
		t.Fatalf("admin should receive user notification unread=%d err=%v", unread, err)
	}
	if _, err := repo.CreateRuntimeNotification(&models.RuntimeNotification{ID: "event-invalid-target", Type: models.NotificationDetectionRunResumed, Level: models.NotificationLevelInfo, TargetType: models.NotificationTargetUser, TargetID: "bad", Message: "bad target", OccurredAt: occurredAt}); err != nil {
		t.Fatal(err)
	}

	project := &models.Project{ProjectCode: "AC-NOTIFY", Name: "Notify Project", Enabled: true}
	if err := repo.CreateProject(project); err != nil {
		t.Fatal(err)
	}
	members, err := repo.ReplaceProjectMembers(project.ID, []models.SysProjectMember{
		{UserID: admin.ID, MemberRole: "owner", NotifyEnabled: true},
		{UserID: operator.ID, MemberRole: "member", NotifyEnabled: false},
	})
	if err != nil || len(members) != 2 {
		t.Fatalf("replace project members len=%d err=%v", len(members), err)
	}
	if members[0].Username == "" || members[0].ProjectID != project.ID {
		t.Fatalf("unexpected project members: %+v", members)
	}
	if _, err := repo.MarkAllNotificationsRead(admin.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MarkAllNotificationsRead(operator.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateRuntimeNotification(&models.RuntimeNotification{
		ID:          "event-project-member-admin",
		Type:        models.NotificationDetectionRunStarted,
		Level:       models.NotificationLevelInfo,
		ProjectID:   project.ID,
		ProjectCode: project.ProjectCode,
		Message:     "project target",
		OccurredAt:  occurredAt.Add(3 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	if unread, err := repo.CountUnreadNotifications(admin.ID); err != nil || unread != 1 {
		t.Fatalf("admin should receive project member notification unread=%d err=%v", unread, err)
	}
	if unread, err := repo.CountUnreadNotifications(operator.ID); err != nil || unread != 0 {
		t.Fatalf("operator has notify disabled unread=%d err=%v", unread, err)
	}
	members, err = repo.ReplaceProjectMembers(project.ID, []models.SysProjectMember{
		{UserID: operator.ID, MemberRole: "operator", NotifyEnabled: true},
	})
	if err != nil || len(members) != 1 || members[0].UserID != operator.ID {
		t.Fatalf("replace project members to operator failed members=%+v err=%v", members, err)
	}
	if _, err := repo.MarkAllNotificationsRead(admin.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MarkAllNotificationsRead(operator.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateRuntimeNotification(&models.RuntimeNotification{
		ID:          "event-project-member-operator",
		Type:        models.NotificationDetectionRunStopped,
		Level:       models.NotificationLevelInfo,
		ProjectID:   project.ID,
		ProjectCode: project.ProjectCode,
		Message:     "project target updated",
		OccurredAt:  occurredAt.Add(4 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	if unread, err := repo.CountUnreadNotifications(admin.ID); err != nil || unread != 0 {
		t.Fatalf("admin should no longer receive project notification unread=%d err=%v", unread, err)
	}
	if unread, err := repo.CountUnreadNotifications(operator.ID); err != nil || unread != 1 {
		t.Fatalf("operator should receive project notification unread=%d err=%v", unread, err)
	}
	if _, err := repo.ReplaceProjectMembers(project.ID, []models.SysProjectMember{{UserID: admin.ID}, {UserID: admin.ID}}); err == nil {
		t.Fatal("expected duplicate project member error")
	}
}

func TestRepositoryTaskRuleMethods(t *testing.T) {
	db := newRepositoryTestDB(t)
	repo := NewRepository(db)
	first := &models.TaskRule{
		ProjectID:       1,
		RuleCode:        " rule-1 ",
		Name:            " First ",
		Enabled:         true,
		TriggerVarID:    100,
		TriggerOperator: " GT ",
		TriggerValue:    "1",
		ActionType:      " DETECTION_START ",
		Priority:        5,
	}
	second := &models.TaskRule{
		ProjectID:    1,
		RuleCode:     "rule-2",
		Name:         "Second",
		Enabled:      true,
		TriggerVarID: 100,
		TriggerValue: "1",
		ActionType:   models.TaskRuleActionDetectionStop,
		Priority:     10,
	}
	disabled := &models.TaskRule{
		ProjectID:    1,
		RuleCode:     "rule-disabled",
		Name:         "Disabled",
		Enabled:      false,
		TriggerVarID: 50,
		TriggerValue: "1",
		ActionType:   models.TaskRuleActionStorageEnable,
	}
	for _, rule := range []*models.TaskRule{first, second, disabled} {
		if err := repo.CreateTaskRule(rule); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Model(&models.TaskRule{}).Where("id = ?", disabled.ID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	rules, err := repo.LoadEnabledTaskRules()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 || rules[0].RuleCode != "rule-2" || rules[1].RuleCode != "rule-1" {
		t.Fatalf("unexpected task rules order: %+v", rules)
	}
	if first.TriggerOperator != models.TaskRuleOperatorGT || first.TriggerEdge != models.TaskRuleEdgeAny || first.ActionType != "detection_start" {
		t.Fatalf("task rule normalization failed: %+v", first)
	}
}

func TestRepositoryGatewayTagProjectAndDetectionMethods(t *testing.T) {
	db := newRepositoryTestDB(t)
	repo := NewRepository(db)
	if err := repo.UpsertGatewaySeeds([]models.GatewayConfig{{ID: 1, Name: "gw", Broker: "tcp://127.0.0.1:1883", ClientID: "c", Topic: "topic", Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	if gateways, err := repo.LoadGateways(); err != nil || len(gateways) != 1 {
		t.Fatalf("load gateways len=%d err=%v", len(gateways), err)
	}
	if configs, err := repo.ListGatewayConfigs(); err != nil || len(configs) != 1 {
		t.Fatalf("list gateways len=%d err=%v", len(configs), err)
	}
	if got, err := repo.GetGatewayConfig(1); err != nil || got.Name != "gw" {
		t.Fatalf("get gateway got=%+v err=%v", got, err)
	}
	if err := repo.CreateGatewayConfig(&models.GatewayConfig{Name: "gw2", Broker: "tcp://127.0.0.1:1883", ClientID: "c2", Topic: "topic2", Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.UpdateGatewayConfig(2, map[string]interface{}{"name": "updated"}); err != nil || got.Name != "updated" {
		t.Fatalf("update gateway got=%+v err=%v", got, err)
	}
	if got, err := repo.UpdateGatewayConfig(2, nil); err != nil || got.ID != 2 {
		t.Fatalf("empty update gateway got=%+v err=%v", got, err)
	}
	if err := repo.DeleteGatewayConfig(2); err != nil {
		t.Fatal(err)
	}

	Project := &models.Project{ProjectCode: "AC-01", Name: "Project", DisplayName: "设备", DisplayNameEN: "Project", DisplayNameJA: "設備", Enabled: true}
	if err := repo.CreateProject(Project); err != nil {
		t.Fatal(err)
	}
	if Projects, err := repo.ListProjects(); err != nil || len(Projects) != 1 || Projects[0].DisplayNameEN != "Project" {
		t.Fatalf("list Projects len=%d err=%v", len(Projects), err)
	}
	legacyProject := &models.Project{ProjectCode: "AC-02", Name: "Legacy Project", Enabled: true}
	if err := repo.CreateProject(legacyProject); err != nil {
		t.Fatal(err)
	}
	ensured, created, updated, err := repo.EnsureProjectByCode(models.Project{ProjectCode: "AC-03", Name: "Ensured", DisplayName: "Ensured", Enabled: false})
	if err != nil || !created || updated || ensured.ID == 0 || !ensured.Enabled {
		t.Fatalf("ensure create got=%+v created=%v updated=%v err=%v", ensured, created, updated, err)
	}
	ensured, created, updated, err = repo.EnsureProjectByCode(models.Project{ProjectCode: "AC-03", SiteNo: "S-3", DisplayNameEN: "Ensured EN", DisplayNameJA: "Ensured JA"})
	if err != nil || created || !updated || ensured.SiteNo != "S-3" || ensured.DisplayNameEN != "Ensured EN" || ensured.DisplayNameJA != "Ensured JA" {
		t.Fatalf("ensure update got=%+v created=%v updated=%v err=%v", ensured, created, updated, err)
	}
	ensured, created, updated, err = repo.EnsureProjectByCode(models.Project{ProjectCode: "AC-03"})
	if err != nil || created || updated || ensured.ProjectCode != "AC-03" {
		t.Fatalf("ensure existing got=%+v created=%v updated=%v err=%v", ensured, created, updated, err)
	}
	if err := repo.EnsureProjectDisplayNameFallbacks(); err != nil {
		t.Fatal(err)
	}
	var reloadedLegacy models.Project
	if err := db.First(&reloadedLegacy, "project_code = ?", "AC-02").Error; err != nil {
		t.Fatal(err)
	}
	if reloadedLegacy.DisplayName != "Legacy Project" {
		t.Fatalf("legacy display fallback failed: %+v", reloadedLegacy)
	}
	updatedProject, err := repo.UpdateProject(Project.ID, map[string]interface{}{"display_name_en": "Updated Project"})
	if err != nil || updatedProject.DisplayNameEN != "Updated Project" {
		t.Fatalf("update Project got=%+v err=%v", updatedProject, err)
	}
	if got, err := repo.UpdateProject(Project.ID, nil); err != nil || got.ID != Project.ID {
		t.Fatalf("empty update Project got=%+v err=%v", got, err)
	}
	defaultLimitH := 32.0
	tag := models.TagConfig{
		VarID:       100,
		GatewayID:   1,
		SourcePath:  "temp",
		RawName:     "temp",
		VarName:     "temp",
		JSONPath:    "temp",
		DataType:    "FLOAT",
		ScaleFactor: 1, DefaultAlarmEnabled: true,
		DefaultLimitH:          &defaultLimitH,
		DefaultLimitDeadband:   0.3,
		DefaultViolationHoldMS: 500,
		DefaultRecoverHoldMS:   600,
		Discovered:             true,
		Enabled:                true,
	}
	if err := repo.CreateTag(&tag); err != nil {
		t.Fatal(err)
	}
	if tags, err := repo.LoadTags(); err != nil || len(tags) != 0 {
		t.Fatalf("load tags len=%d err=%v", len(tags), err)
	}
	enabled := true
	if tags, err := repo.ListTags(TagFilter{GatewayID: intPtr(1), Enabled: &enabled, Keyword: "temp"}); err != nil || len(tags) != 1 {
		t.Fatalf("list tags len=%d err=%v", len(tags), err)
	}
	if got, err := repo.UpdateTag(100, map[string]interface{}{"display_name": "Temperature"}); err != nil || got.DisplayName != "Temperature" {
		t.Fatalf("update tag got=%+v err=%v", got, err)
	}
	if got, err := repo.UpdateTag(100, nil); err != nil || got.VarID != 100 {
		t.Fatalf("empty update tag got=%+v err=%v", got, err)
	}
	if err := repo.AssignTag(100, &Project.ID, "", "group", true); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdateTag(100, map[string]interface{}{
		"rw_mode":         models.RWModeReadWrite,
		"writable":        true,
		"write_source_id": 1,
		"write_path":      "temp_set",
		"write_data_type": "FLOAT",
	}); err != nil {
		t.Fatal(err)
	}
	routes, err := repo.ListStorageRoutesByProject(Project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 1 || routes[0].VarID != 100 || routes[0].StorageTarget != models.StorageTargetWideTable || routes[0].StorageTable != ProjectWideTableName(Project.ID) || routes[0].ColumnName != "temp" || routes[0].Enabled {
		t.Fatalf("expected default wide-table storage route, got %+v", routes)
	}
	if err := db.Model(&models.StorageRoute{}).Where("id = ?", routes[0].ID).Updates(map[string]interface{}{
		"enabled":        true,
		"trigger_mode":   models.StoreTriggerOnCycle,
		"cycle_ms":       3000,
		"store_on_start": true,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if tags, err := repo.LoadTags(); err != nil || len(tags) != 1 || tags[0].VarID != 100 {
		t.Fatalf("load assigned runtime tags got=%+v err=%v", tags, err)
	}
	if err := repo.UpsertDiscoveredTags([]models.TagConfig{{VarID: 101, GatewayID: 1, SourcePath: "pressure", RawName: "pressure", VarName: "pressure", JSONPath: "pressure", DataType: "FLOAT", Enabled: true, ScaleFactor: 1}}); err != nil {
		t.Fatal(err)
	}
	discoveredTag, err := repo.GetTag(101)
	if err != nil {
		t.Fatal(err)
	}
	if discoveredTag.Enabled || !discoveredTag.Discovered || discoveredTag.Writable || discoveredTag.RWMode != models.RWModeRead {
		t.Fatalf("unexpected discovered tag defaults: %+v", discoveredTag)
	}
	if tags, err := repo.LoadTags(); err != nil || len(tags) != 1 || tags[0].VarID != 100 {
		t.Fatalf("discovered candidate should not load into runtime tags got=%+v err=%v", tags, err)
	}
	if err := repo.UpsertDiscoveredTags([]models.TagConfig{{VarID: 999, GatewayID: 1, SourceTopic: "topic-updated", SourcePath: "temp", RawName: "temp", VarName: "temp", JSONPath: "temp", DataType: "FLOAT", Enabled: false, ScaleFactor: 1}}); err != nil {
		t.Fatal(err)
	}
	assignedTag, err := repo.GetTag(100)
	if err != nil {
		t.Fatal(err)
	}
	if !assignedTag.Enabled || assignedTag.ProjectID == nil || assignedTag.SourceTopic != "topic-updated" || !assignedTag.Writable || assignedTag.RWMode != models.RWModeReadWrite || assignedTag.WritePath != "temp_set" {
		t.Fatalf("existing assigned tag should keep runtime eligibility while refreshing source metadata: %+v", assignedTag)
	}
	if err := repo.DeleteTag(101); err != nil {
		t.Fatal(err)
	}

	limitH := 30.0
	standard := &models.DetectionStandard{StandardCode: "STD-1", Name: "Standard", ProjectID: &Project.ID, ProjectCode: Project.ProjectCode, Mode: "standard", Enabled: true}
	if err := repo.CreateDetectionStandard(standard, []models.DetectionStandardItem{
		{VarID: 100, VarName: "temp", DisplayName: "温度", CheckEnabled: true, StoreEnabled: true, Required: true, LimitH: &limitH, Unit: "C", DecimalPlaces: 1},
		{VarID: 102, VarName: "label", CheckEnabled: false, StoreEnabled: false},
	}); err != nil {
		t.Fatal(err)
	}
	if standards, err := repo.ListDetectionStandards(DetectionStandardFilter{Keyword: "STD"}); err != nil || len(standards) != 1 {
		t.Fatalf("list standards len=%d err=%v", len(standards), err)
	}
	gotStandard, err := repo.GetDetectionStandard(standard.ID)
	if err != nil || len(gotStandard.Items) != 2 {
		t.Fatalf("get standard got=%+v err=%v", gotStandard, err)
	}
	gotStandard, err = repo.ReplaceDetectionStandardItems(standard.ID, []models.DetectionStandardItem{{VarID: 100, VarName: "temp", CheckEnabled: true, AlarmEnabled: true, StoreEnabled: true, CheckOnStart: true}})
	if err != nil || gotStandard.Version != 2 || len(gotStandard.Items) != 1 {
		t.Fatalf("replace standard items got=%+v err=%v", gotStandard, err)
	}
	if gotStandard.Items[0].CheckMethod != models.CheckMethodNumericRange || gotStandard.Items[0].QualityPolicy != models.QualityPolicyIgnoreBad {
		t.Fatalf("expected standard item defaults, got %+v", gotStandard.Items[0])
	}
	gotStandard, err = repo.UpdateDetectionStandard(standard.ID, map[string]interface{}{"display_name": "检测标准"})
	if err != nil || gotStandard.DisplayName != "检测标准" {
		t.Fatalf("update standard got=%+v err=%v", gotStandard, err)
	}
	limitCheck := true
	task, err := repo.StartDetectionTaskWithOptions(StartDetectionOptions{ProjectID: Project.ID, TestNo: "T-1", Mode: "standard", StandardID: &standard.ID, LimitCheckEnabled: &limitCheck, EndPolicy: models.DetectionEndPolicyFixedDuration, DurationSec: 60, StartedByUserID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if task.StandardID == nil || *task.StandardID != standard.ID || len(task.StandardItems) != 1 || !task.LimitCheckEnabled || task.EndPolicy != models.DetectionEndPolicyFixedDuration {
		t.Fatalf("expected standard snapshot on task: %+v", task)
	}
	if task.StandardItems[0].CheckMethod != models.CheckMethodNumericRange || task.StandardItems[0].QualityPolicy != models.QualityPolicyIgnoreBad || !task.StandardItems[0].AlarmEnabled || !task.StandardItems[0].CheckOnStart || task.StandardItems[0].CheckCycleMS != 0 {
		t.Fatalf("expected standard snapshot to freeze check config, got %+v", task.StandardItems[0])
	}
	if !task.StandardItems[0].VariableDefaultAlarmEnabled || task.StandardItems[0].VariableDefaultLimitH == nil || *task.StandardItems[0].VariableDefaultLimitH != defaultLimitH || task.StandardItems[0].VariableDefaultLimitDeadband != 0.3 || task.StandardItems[0].VariableDefaultViolationHoldMS != 500 || task.StandardItems[0].VariableDefaultRecoverHoldMS != 600 {
		t.Fatalf("expected standard snapshot to freeze variable defaults, got %+v", task.StandardItems[0])
	}
	runRoutes, err := repo.ListRunStorageRoutes(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runRoutes) != 1 || runRoutes[0].TaskID != task.ID || runRoutes[0].VarID != 100 || runRoutes[0].StorageTarget != models.StorageTargetWideTable || runRoutes[0].StorageTable != ProjectWideTableName(Project.ID) || runRoutes[0].ColumnName != "temp" || runRoutes[0].CycleMS != 3000 || !runRoutes[0].StoreOnStart {
		t.Fatalf("expected run storage route snapshot, got %+v", runRoutes)
	}
	if !db.Migrator().HasTable(ProjectWideTableName(Project.ID)) || !db.Migrator().HasColumn(ProjectWideTableName(Project.ID), "temp") {
		t.Fatal("expected Project wide table and dynamic column for routes")
	}
	if _, err := repo.StartDetectionTask(Project.ID, "T-2", "standard", nil); err == nil {
		t.Fatal("expected duplicate running task error")
	}
	if tasks, err := repo.LoadActiveDetectionTasks(); err != nil || len(tasks) != 1 {
		t.Fatalf("active tasks len=%d err=%v", len(tasks), err)
	}
	paused, err := repo.PauseDetectionTask(task.ID, "operator pause")
	if err != nil || paused.Status != models.DetectionStatusPaused {
		t.Fatalf("pause task got=%+v err=%v", paused, err)
	}
	if paused.PauseStartedAt == nil {
		t.Fatalf("expected pause start time, got %+v", paused)
	}
	if tasks, err := repo.LoadActiveDetectionTasks(); err != nil || len(tasks) != 0 {
		t.Fatalf("paused task should not be runtime active len=%d err=%v", len(tasks), err)
	}
	pausedSince := time.Now().Add(-10 * time.Second)
	originalExpectedEnd := *task.ExpectedEndAt
	if err := db.Model(&models.DetectionTask{}).Where("id = ?", task.ID).Update("pause_started_at", pausedSince).Error; err != nil {
		t.Fatal(err)
	}
	resumed, err := repo.ResumeDetectionTask(task.ID)
	if err != nil || resumed.Status != models.DetectionStatusRunning || len(resumed.StorageRoutes) != 1 || len(resumed.StandardItems) != 1 {
		t.Fatalf("resume task got=%+v err=%v", resumed, err)
	}
	if resumed.PausedDurationMS < 9900 || resumed.ExpectedEndAt == nil || resumed.ExpectedEndAt.Sub(originalExpectedEnd) < 9900*time.Millisecond {
		t.Fatalf("expected pause duration to shift expected end, got %+v original_expected=%s", resumed, originalExpectedEnd)
	}
	updatedLimitH := 28.0
	tag.DefaultLimitH = &updatedLimitH
	tag.DefaultViolationHoldMS = 700
	rows, err := repo.UpdateRunningRunItemsVariableDefaults(100, tag)
	if err != nil || rows != 1 {
		t.Fatalf("update running variable defaults rows=%d err=%v", rows, err)
	}
	taskDetail, err := repo.GetDetectionTask(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if taskDetail.StandardItems[0].VariableDefaultLimitH == nil || *taskDetail.StandardItems[0].VariableDefaultLimitH != updatedLimitH || taskDetail.StandardItems[0].VariableDefaultViolationHoldMS != 700 {
		t.Fatalf("expected running snapshot variable default update, got %+v", taskDetail.StandardItems[0])
	}
	stopped, err := repo.StopDetectionTask(task.ID, "done")
	if err != nil || stopped.Status != "stopped" {
		t.Fatalf("stop task got=%+v err=%v", stopped, err)
	}
	if err := repo.InsertHistoryBatch([]*models.StoreTask{{GatewayID: 1, Topic: "topic", ProjectID: Project.ID, TaskID: task.ID, TestNo: "T-1", VarID: 100, VarName: "temp", ProjectCode: "AC-01", Value: 23.5, Quality: 1, Timestamp: time.Now()}}); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertHistoryBatch([]*models.StoreTask{{GatewayID: 1, Topic: "topic", ProjectID: Project.ID, TaskID: task.ID, TestNo: "T-1", VarID: 102, VarName: "label", ProjectCode: "AC-01", StrValue: "ok", IsString: true, Quality: 1, Timestamp: time.Now()}}); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertHistoryBatch(nil); err != nil {
		t.Fatal(err)
	}
	features, err := repo.RefreshDetectionRunFeatures(task.ID)
	if err != nil || len(features) != 1 || features[0].VarID != 100 || features[0].SampleCount != 1 || features[0].AvgValue == nil || *features[0].AvgValue != 23.5 {
		t.Fatalf("unexpected detection features: %+v err=%v", features, err)
	}
	if err := repo.DeleteDetectionStandard(standard.ID); !errors.Is(err, ErrReferenced) {
		t.Fatalf("expected referenced standard delete protection, got %v", err)
	}
	if err := repo.DeleteDetectionStandard(standard.ID + 999); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected missing standard delete to return record not found, got %v", err)
	}
	if err := repo.SetDetectionStandardFavorite(1, standard.ID+999, true); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected missing standard favorite to return record not found, got %v", err)
	}
	if err := repo.SetDetectionStandardFavorite(1, standard.ID+999, false); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected missing standard unfavorite to return record not found, got %v", err)
	}
	if err := repo.SetDetectionStandardFavorite(1, standard.ID, true); err != nil {
		t.Fatal(err)
	}
	favorites, err := repo.ListFavoriteDetectionStandards(1)
	if err != nil || len(favorites) != 1 || favorites[0].ID != standard.ID {
		t.Fatalf("unexpected favorites: %+v err=%v", favorites, err)
	}
	recents, err := repo.ListRecentDetectionStandards(1, &Project.ID, 10)
	if err != nil || len(recents) != 1 || recents[0].ID != standard.ID {
		t.Fatalf("unexpected recents: %+v err=%v", recents, err)
	}
	if err := repo.SetDetectionStandardFavorite(1, standard.ID, false); err != nil {
		t.Fatal(err)
	}
	favorites, err = repo.ListFavoriteDetectionStandards(1)
	if err != nil || len(favorites) != 0 {
		t.Fatalf("expected favorite removed, got %+v err=%v", favorites, err)
	}
	limitCheck = false
	customTask, err := repo.StartDetectionTaskWithOptions(StartDetectionOptions{
		ProjectID:         Project.ID,
		TestNo:            "T-CUSTOM",
		Mode:              "custom",
		LimitCheckEnabled: &limitCheck,
		EndPolicy:         models.DetectionEndPolicyManual,
		CustomItems: []models.DetectionStandardItem{{
			VarID:        100,
			VarName:      "temp",
			CheckEnabled: true,
			AlarmEnabled: true,
			StoreEnabled: true,
			LimitH:       &limitH,
		}},
		ReportRequest: map[string]any{
			"report_name": "default variable report",
			"ext_2":       "global-ext",
			"variables": []any{map[string]any{
				"var_id":      "100",
				"report_name": "temp report",
				"ext_1":       "operator-note",
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if customTask.StandardID != nil || customTask.StandardCode != "custom" || customTask.CustomConfigJSON == "" || customTask.LimitCheckEnabled || strings.Contains(customTask.CustomConfigJSON, "created_at") {
		t.Fatalf("unexpected custom task: %+v", customTask)
	}
	if !strings.Contains(customTask.CustomConfigJSON, `"report_request"`) {
		t.Fatalf("expected report request frozen in custom_config_json=%s", customTask.CustomConfigJSON)
	}
	if len(customTask.StandardItems) != 1 || customTask.StandardItems[0].CheckEnabled || customTask.StandardItems[0].AlarmEnabled {
		t.Fatalf("limit_check_enabled=false should disable check/alarm in snapshot: %+v", customTask.StandardItems)
	}
	if len(customTask.ReportRequests) != 1 || customTask.ReportRequests[0].VarID != 100 || customTask.ReportRequests[0].VarName != "temp" || customTask.ReportRequests[0].ReportName != "temp report" || customTask.ReportRequests[0].Ext1 != "operator-note" || customTask.ReportRequests[0].Ext2 != "global-ext" {
		t.Fatalf("unexpected report request snapshot: %+v", customTask.ReportRequests)
	}
	customDetail, err := repo.GetDetectionTask(customTask.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(customDetail.ReportRequests) != 1 || customDetail.ReportRequests[0].DisplayName != "Temperature" {
		t.Fatalf("expected report requests attached to task detail: %+v", customDetail.ReportRequests)
	}
	if _, err := repo.StopDetectionTask(customTask.ID, "custom done"); err != nil {
		t.Fatal(err)
	}

	templates, err := repo.ListReportTemplates(ReportTemplateFilter{})
	if err != nil || len(templates) != 0 {
		t.Fatalf("unexpected templates len=%d err=%v", len(templates), err)
	}
	template := &models.ReportTemplate{TemplateCode: "RPT-1", Name: "Report", FileRef: "reports/rpt-1.xlsx", Enabled: true}
	if err := repo.CreateReportTemplate(template); err != nil {
		t.Fatal(err)
	}
	enabledTemplate := true
	templates, err = repo.ListReportTemplates(ReportTemplateFilter{Enabled: &enabledTemplate, Keyword: "RPT"})
	if err != nil || len(templates) != 1 {
		t.Fatalf("templates len=%d err=%v", len(templates), err)
	}
	if got, err := repo.UpdateReportTemplate(template.ID, map[string]interface{}{"remark": "updated"}); err != nil || got.Remark != "updated" {
		t.Fatalf("update template got=%+v err=%v", got, err)
	}
	reportTag := models.TagConfig{
		VarID:       103,
		GatewayID:   1,
		SourcePath:  "humidity",
		RawName:     "humidity",
		VarName:     "humidity",
		DisplayName: "Humidity",
		JSONPath:    "humidity",
		DataType:    "FLOAT",
		ScaleFactor: 1,
		ProjectID:   &Project.ID,
		ProjectCode: Project.ProjectCode,
		Discovered:  true,
		Enabled:     true,
	}
	if err := repo.CreateTag(&reportTag); err != nil {
		t.Fatal(err)
	}
	reportTask, err := repo.StartDetectionTaskWithOptions(StartDetectionOptions{
		ProjectID: Project.ID,
		TestNo:    "T-REPORT",
		Mode:      "custom",
		CustomItems: []models.DetectionStandardItem{
			{VarID: 100, VarName: "temp", CheckEnabled: true, AlarmEnabled: true, StoreEnabled: true},
			{VarID: reportTag.VarID, VarName: reportTag.VarName, CheckEnabled: true, AlarmEnabled: true, StoreEnabled: true},
		},
		ReportRequest: map[string]any{
			"reports": []any{map[string]any{
				"template_id":   template.ID,
				"report_name":   "performance report",
				"variables":     []any{map[string]any{"var_id": "100"}, map[string]any{"var_name": "humidity"}},
				"params":        map[string]any{"inlet_area_m2": 1.25, "remark": "formula input"},
				"template_code": "ignored-because-id-wins",
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(reportTask.ReportRequests) != 1 || reportTask.ReportRequests[0].TemplateID == nil || *reportTask.ReportRequests[0].TemplateID != template.ID || reportTask.ReportRequests[0].TemplateCode != template.TemplateCode || !strings.Contains(reportTask.ReportRequests[0].VariablesJSON, `"humidity"`) || !strings.Contains(reportTask.ReportRequests[0].ParamsJSON, `"inlet_area_m2":1.25`) {
		t.Fatalf("unexpected report request row: %+v", reportTask.ReportRequests)
	}
	if _, err := repo.StopDetectionTask(reportTask.ID, "report done"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateDetectionRunReport(&models.DetectionRunReport{TaskID: task.ID, TemplateID: &template.ID, TemplateCode: template.TemplateCode, TemplateVersion: template.Version, FileRef: "reports/out.xlsx", Status: "generated"}); err != nil {
		t.Fatal(err)
	}
	if reports, err := repo.ListDetectionRunReports(task.ID); err != nil || len(reports) != 1 {
		t.Fatalf("reports len=%d err=%v", len(reports), err)
	}
	if err := repo.DeleteReportTemplate(template.ID); !errors.Is(err, ErrReferenced) {
		t.Fatalf("expected referenced template delete protection, got %v", err)
	}
	if err := repo.DeleteReportTemplate(template.ID + 999); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected missing template delete to return record not found, got %v", err)
	}

	if err := repo.CreateDetectionRunNote(&models.DetectionRunNote{TaskID: task.ID, Content: "note", NoteType: "memo"}); err != nil {
		t.Fatal(err)
	}
	if notes, err := repo.ListDetectionRunNotes(task.ID, 10); err != nil || len(notes) != 1 {
		t.Fatalf("notes len=%d err=%v", len(notes), err)
	}
	if tasks, err := repo.ListDetectionTasks(DetectionTaskFilter{ProjectID: &Project.ID, Status: models.DetectionStatusStopped}); err != nil || len(tasks) != 3 {
		t.Fatalf("tasks len=%d err=%v", len(tasks), err)
	}
	if got, err := repo.GetDetectionTask(task.ID); err != nil || got.ID != task.ID || len(got.RecentNotes) != 1 || len(got.Reports) != 1 {
		t.Fatalf("task detail got=%+v err=%v", got, err)
	}
	if rows, err := repo.QueryHistoryData(HistoryFilter{TaskID: &task.ID}); err != nil || len(rows) != 2 {
		t.Fatalf("history by task rows=%d err=%v", len(rows), err)
	}
	if !db.Migrator().HasTable(&models.DetectionLimitAlarm{}) {
		t.Fatal("expected detection limit alarm table to be auto migrated")
	}
	if !db.Migrator().HasColumn(&models.DetectionLimitAlarm{}, "scope") {
		t.Fatal("expected detection limit alarm scope column to be auto migrated")
	}
	if !db.Migrator().HasTable(&models.StorageRoute{}) || !db.Migrator().HasTable(&models.DetectionRunStorageRoute{}) {
		t.Fatal("expected storage route tables to be auto migrated")
	}
	if !db.Migrator().HasTable(&models.DetectionRunEvent{}) || !db.Migrator().HasTable(&models.DetectionRunSummary{}) || !db.Migrator().HasTable(&models.DetectionRunFeature{}) {
		t.Fatal("expected detection run event, summary, and feature tables to be auto migrated")
	}
	if err := repo.CreateDetectionRunEvent(&models.DetectionRunEvent{TaskID: task.ID, TestNo: task.TestNo, ProjectID: Project.ID, ProjectCode: Project.ProjectCode, EventType: models.DetectionEventRunStarted, EventLevel: "info", Message: "started"}); err != nil {
		t.Fatal(err)
	}
	if events, err := repo.ListDetectionRunEvents(task.ID, 10); err != nil || len(events) != 1 || events[0].EventType != models.DetectionEventRunStarted {
		t.Fatalf("events=%+v err=%v", events, err)
	}
	startValue := 35.0
	limitValue := 30.0
	startedAt := time.Now()
	if err := repo.CreateDetectionLimitAlarm(&models.DetectionLimitAlarm{
		TaskID:        task.ID,
		TestNo:        task.TestNo,
		ProjectID:     Project.ID,
		ProjectCode:   Project.ProjectCode,
		StandardID:    task.StandardID,
		VarID:         100,
		VarName:       "temp",
		CheckMethod:   models.CheckMethodNumericRange,
		AlarmType:     "above_h",
		AlarmLevel:    "H",
		Status:        models.DetectionAlarmStatusActive,
		StartValue:    &startValue,
		PeakValue:     &startValue,
		LimitValue:    &limitValue,
		Quality:       1,
		FirstSeenAt:   startedAt,
		LastSeenAt:    startedAt,
		LimitDeadband: 1,
	}); err != nil {
		t.Fatal(err)
	}
	recoverValue := 28.0
	recoveredAt := startedAt.Add(time.Second)
	if err := repo.RecoverDetectionLimitAlarm(&models.DetectionLimitAlarmEvent{
		Action: models.DetectionAlarmActionRecover,
		Alarm: models.DetectionLimitAlarm{
			TaskID:       task.ID,
			VarID:        100,
			AlarmType:    "above_h",
			PeakValue:    &startValue,
			RecoverValue: &recoverValue,
			Quality:      1,
			LastSeenAt:   recoveredAt,
			RecoveredAt:  &recoveredAt,
			DurationMS:   1000,
		},
	}); err != nil {
		t.Fatal(err)
	}
	var alarm models.DetectionLimitAlarm
	if err := db.First(&alarm, "task_id = ? AND var_id = ? AND alarm_type = ?", task.ID, 100, "above_h").Error; err != nil {
		t.Fatal(err)
	}
	if alarm.Scope != models.AlarmScopeDetection || alarm.Status != models.DetectionAlarmStatusClosed || alarm.RecoverValue == nil || *alarm.RecoverValue != recoverValue {
		t.Fatalf("expected recovered detection limit alarm, got %+v", alarm)
	}
	defaultStartValue := 5.0
	defaultLimitValue := 10.0
	if err := repo.CreateDetectionLimitAlarm(&models.DetectionLimitAlarm{
		Scope:         models.AlarmScopeDefault,
		TaskID:        0,
		ProjectID:     Project.ID,
		ProjectCode:   Project.ProjectCode,
		VarID:         101,
		VarName:       "pressure",
		CheckMethod:   models.CheckMethodNumericRange,
		AlarmType:     "below_l",
		AlarmLevel:    "L",
		Status:        models.DetectionAlarmStatusActive,
		StartValue:    &defaultStartValue,
		PeakValue:     &defaultStartValue,
		LimitValue:    &defaultLimitValue,
		Quality:       1,
		FirstSeenAt:   startedAt,
		LastSeenAt:    startedAt,
		LimitDeadband: 1,
	}); err != nil {
		t.Fatal(err)
	}
	defaultRecoverValue := 11.5
	if err := repo.RecoverDetectionLimitAlarm(&models.DetectionLimitAlarmEvent{
		Action: models.DetectionAlarmActionRecover,
		Alarm: models.DetectionLimitAlarm{
			Scope:        models.AlarmScopeDefault,
			TaskID:       0,
			VarID:        101,
			AlarmType:    "below_l",
			PeakValue:    &defaultStartValue,
			RecoverValue: &defaultRecoverValue,
			Quality:      1,
			LastSeenAt:   recoveredAt,
			RecoveredAt:  &recoveredAt,
			DurationMS:   1000,
		},
	}); err != nil {
		t.Fatal(err)
	}
	var defaultAlarm models.DetectionLimitAlarm
	if err := db.First(&defaultAlarm, "scope = ? AND task_id = ? AND var_id = ?", models.AlarmScopeDefault, 0, 101).Error; err != nil {
		t.Fatal(err)
	}
	if defaultAlarm.Status != models.DetectionAlarmStatusClosed || defaultAlarm.RecoverValue == nil || *defaultAlarm.RecoverValue != defaultRecoverValue {
		t.Fatalf("expected recovered default limit alarm, got %+v", defaultAlarm)
	}
	defaultAlarms, total, err := repo.ListLimitAlarms(LimitAlarmFilter{Scope: models.AlarmScopeDefault, ProjectID: &Project.ID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(defaultAlarms) != 1 || defaultAlarms[0].Scope != models.AlarmScopeDefault || defaultAlarms[0].TaskID != 0 {
		t.Fatalf("unexpected default alarm list total=%d items=%+v", total, defaultAlarms)
	}
	detectionAlarms, detectionTotal, err := repo.ListLimitAlarms(LimitAlarmFilter{Scope: models.AlarmScopeDetection, TaskID: &task.ID, Status: models.DetectionAlarmStatusClosed, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if detectionTotal != 1 || len(detectionAlarms) != 1 || detectionAlarms[0].TaskID != task.ID {
		t.Fatalf("unexpected detection alarm list total=%d items=%+v", detectionTotal, detectionAlarms)
	}
	summary, err := repo.RefreshDetectionRunSummary(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.ResultStatus != models.DetectionSummaryStatusNG || summary.HistoryRows != 2 || summary.AlarmTotal != 1 || summary.AlarmRecovered != 1 || summary.AlarmAboveH != 1 {
		t.Fatalf("unexpected detection summary: %+v", summary)
	}
}

func TestDetectionStandardItemsFreezeVariableDisplaySnapshot(t *testing.T) {
	db := newRepositoryTestDB(t)
	repo := NewRepository(db)
	project := &models.Project{ProjectCode: "AC-SNAP", Name: "Snapshot Project", Enabled: true}
	if err := repo.CreateProject(project); err != nil {
		t.Fatal(err)
	}
	tag := models.TagConfig{
		VarID:         91001,
		GatewayID:     1,
		SourcePath:    "temp",
		RawName:       "temp",
		VarName:       "temp",
		DisplayName:   "温度",
		DisplayNameEN: "Temperature",
		DisplayNameJA: "温度",
		JSONPath:      "temp",
		DataType:      "FLOAT",
		Unit:          "C",
		DecimalPlaces: 1,
		ScaleFactor:   1,
		ProjectID:     &project.ID,
		ProjectCode:   project.ProjectCode,
		Discovered:    true,
		Enabled:       true,
	}
	if err := repo.CreateTag(&tag); err != nil {
		t.Fatal(err)
	}
	standard := &models.DetectionStandard{StandardCode: "STD-SNAPSHOT", Name: "Snapshot", ProjectID: &project.ID, ProjectCode: project.ProjectCode, Mode: "standard", Enabled: true}
	if err := repo.CreateDetectionStandard(standard, []models.DetectionStandardItem{{
		VarID:        tag.VarID,
		VarName:      tag.VarName,
		CheckEnabled: true,
		AlarmEnabled: true,
		StoreEnabled: true,
		CheckOnStart: true,
	}}); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetDetectionStandard(standard.ID)
	if err != nil || len(got.Items) != 1 {
		t.Fatalf("get standard got=%+v err=%v", got, err)
	}
	if got.Items[0].DisplayName != "温度" || got.Items[0].DisplayNameEN != "Temperature" || got.Items[0].DisplayNameJA != "温度" || got.Items[0].Unit != "C" {
		t.Fatalf("expected item display snapshot from tag, got %+v", got.Items[0])
	}
	if _, err := repo.ReplaceDetectionStandardItems(standard.ID, []models.DetectionStandardItem{{VarID: tag.VarID, VarName: tag.VarName, CheckMethod: "bad"}}); err == nil {
		t.Fatal("expected invalid check_method to be rejected")
	}
	if _, err := repo.ReplaceDetectionStandardItems(standard.ID, []models.DetectionStandardItem{{VarID: tag.VarID, VarName: tag.VarName, QualityPolicy: "bad"}}); err == nil {
		t.Fatal("expected invalid quality_policy to be rejected")
	}
	task, err := repo.StartDetectionTaskWithOptions(StartDetectionOptions{ProjectID: project.ID, TestNo: "SNAP-1", Mode: "standard", StandardID: &standard.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdateTag(tag.VarID, map[string]interface{}{"display_name": "改名后温度", "display_name_en": "Renamed Temperature", "display_name_ja": "変更後温度"}); err != nil {
		t.Fatal(err)
	}
	detail, err := repo.GetDetectionTask(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.StandardItems) != 1 || detail.StandardItems[0].DisplayName != "温度" || detail.StandardItems[0].DisplayNameEN != "Temperature" || detail.StandardItems[0].DisplayNameJA != "温度" {
		t.Fatalf("expected run item to keep original display snapshot, got %+v", detail.StandardItems)
	}
}

func TestRepositoryStationViewEffectiveSelectionAndBindings(t *testing.T) {
	db := newRepositoryTestDB(t)
	repo := NewRepository(db)

	project := &models.Project{ProjectCode: "AC-SV-01", Name: "Station View 1", DisplayName: "工位一", ModelName: "KFR", EdgeInstanceID: "edge-a", Enabled: true}
	if err := repo.CreateProject(project); err != nil {
		t.Fatal(err)
	}
	otherProject := &models.Project{ProjectCode: "AC-SV-02", Name: "Station View 2", EdgeInstanceID: "edge-a", Enabled: true}
	if err := repo.CreateProject(otherProject); err != nil {
		t.Fatal(err)
	}
	limitL := 10.0
	limitH := 20.0
	tags := []models.TagConfig{
		{VarID: 22, GatewayID: 1, SourcePath: "sv/project-1/humidity", ProjectID: &project.ID, ProjectCode: project.ProjectCode, VarName: "humidity", VarGroup: "air", DisplayName: "湿度", DataType: "FLOAT", Unit: "%", DecimalPlaces: 1, Enabled: true},
		{VarID: 11, GatewayID: 1, SourcePath: "sv/project-1/temp", ProjectID: &project.ID, ProjectCode: project.ProjectCode, VarName: "temp", VarGroup: "air", DisplayName: "温度", DisplayNameEN: "Temperature", DataType: "FLOAT", Unit: "C", DecimalPlaces: 2, DefaultLimitL: &limitL, DefaultLimitH: &limitH, DefaultAlarmEnabled: true, Enabled: true},
		{VarID: 99, GatewayID: 1, SourcePath: "sv/project-2/temp", ProjectID: &otherProject.ID, ProjectCode: otherProject.ProjectCode, VarName: "temp", VarGroup: "air", DisplayName: "其他项目温度", DataType: "FLOAT", Enabled: true},
	}
	if err := db.Create(&tags).Error; err != nil {
		t.Fatal(err)
	}

	defaultView, err := repo.GetEffectiveStationView(project.ID, "edge-a")
	if err != nil {
		t.Fatal(err)
	}
	if defaultView.Template.TemplateCode != "STATION-DEFAULT" || len(defaultView.Regions) != 2 {
		t.Fatalf("default view not seeded: %+v", defaultView)
	}
	if got := strings.Join(defaultView.WSSubscription.VarIDs, ","); got != "11,22" {
		t.Fatalf("default view should only subscribe current project tags, got %s", got)
	}
	if !defaultView.HTTPCompanion.CurrentRunRequired || len(defaultView.Warnings) == 0 {
		t.Fatalf("default view should require current run and warn when absent: %+v warnings=%v", defaultView.HTTPCompanion, defaultView.Warnings)
	}
	if _, err := repo.GetEffectiveStationView(project.ID, "edge-b"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("edge mismatch should be not found, got %v", err)
	}

	customTemplate := models.StationViewTemplate{
		TemplateUID:  "station-custom",
		TemplateCode: "STATION-CUSTOM",
		Name:         "Custom station view",
		Version:      3,
		Status:       models.StationViewStatusPublished,
		OwnerScope:   "edge",
	}
	customRegions := []models.StationViewRegion{
		{TemplateUID: customTemplate.TemplateUID, RegionKey: "left", RegionType: "metric_grid", SortOrder: 1, Enabled: true},
		{TemplateUID: customTemplate.TemplateUID, RegionKey: "right", RegionType: "inspection_table", SortOrder: 2, Enabled: true},
	}
	customItems := []models.StationViewItem{
		{TemplateUID: customTemplate.TemplateUID, RegionKey: "left", ItemUID: "custom-temp", ItemType: "metric_card", BindingType: models.StationViewBindingVarName, BindingKey: "temp", SortOrder: 10, Visible: true},
		{TemplateUID: customTemplate.TemplateUID, RegionKey: "right", ItemUID: "custom-run-items", ItemType: "inspection_row", BindingType: models.StationViewBindingDetectionItems, SortOrder: 20, Visible: true},
	}
	customAssignment := models.StationViewAssignment{
		TemplateUID: customTemplate.TemplateUID,
		TargetType:  models.StationViewTargetProject,
		TargetKey:   project.ProjectCode,
		Priority:    10,
		Enabled:     true,
	}
	if err := db.Create(&customTemplate).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&customRegions).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&customItems).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&customAssignment).Error; err != nil {
		t.Fatal(err)
	}

	started := time.Now().Add(-time.Minute)
	task := models.DetectionTask{
		TestNo:      "SV-RUN-1",
		ProjectID:   project.ID,
		ProjectCode: project.ProjectCode,
		Mode:        "standard",
		Status:      models.DetectionStatusPaused,
		StartedAt:   &started,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	runItems := []models.DetectionRunStandardItem{
		{TaskID: task.ID, TestNo: task.TestNo, VarID: 33, VarName: "pressure", DisplayName: "压力", Unit: "Pa", DecimalPlaces: 0, CheckEnabled: true, AlarmEnabled: true, SortOrder: 30},
		{TaskID: task.ID, TestNo: task.TestNo, VarID: 11, VarName: "temp", DisplayName: "运行温度", Unit: "C", DecimalPlaces: 2, CheckEnabled: true, AlarmEnabled: true, SortOrder: 10},
	}
	if err := db.Create(&runItems).Error; err != nil {
		t.Fatal(err)
	}

	effective, err := repo.GetEffectiveStationView(project.ID, "edge-a")
	if err != nil {
		t.Fatal(err)
	}
	if effective.Template.TemplateCode != "STATION-CUSTOM" || effective.Template.Version != 3 {
		t.Fatalf("project assignment should select custom template, got %+v", effective.Template)
	}
	if len(effective.Items) != 2 || len(effective.Items[0].ResolvedBindings) != 1 || effective.Items[0].ResolvedBindings[0].VarID != 11 {
		t.Fatalf("var_name item should bind only project temp tag: %+v", effective.Items)
	}
	runBindings := effective.Items[1].ResolvedBindings
	if len(runBindings) != 2 || runBindings[0].VarID != 11 || runBindings[1].VarID != 33 {
		t.Fatalf("run bindings should use current paused run sorted by sort_order: %+v", runBindings)
	}
	if got := strings.Join(effective.WSSubscription.VarIDs, ","); got != "11,33" {
		t.Fatalf("effective ws var ids should include resolved card and run items, got %s", got)
	}
}

func TestStationViewAssignmentScore(t *testing.T) {
	project := models.Project{ID: 7, ProjectCode: "AC-SCORE", ModelName: "MODEL-A"}
	cases := []struct {
		name       string
		assignment models.StationViewAssignment
		edge       string
		want       int
	}{
		{name: "project code", assignment: models.StationViewAssignment{TargetType: models.StationViewTargetProject, TargetKey: "AC-SCORE"}, want: 400},
		{name: "project id", assignment: models.StationViewAssignment{TargetType: models.StationViewTargetProject, TargetKey: "7"}, want: 400},
		{name: "edge", assignment: models.StationViewAssignment{TargetType: models.StationViewTargetEdge, TargetKey: "edge-a"}, edge: "edge-a", want: 300},
		{name: "model", assignment: models.StationViewAssignment{TargetType: models.StationViewTargetModel, TargetKey: "MODEL-A"}, want: 200},
		{name: "global", assignment: models.StationViewAssignment{TargetType: models.StationViewTargetGlobal, TargetKey: "*"}, want: 100},
		{name: "miss", assignment: models.StationViewAssignment{TargetType: models.StationViewTargetProject, TargetKey: "other"}, want: 0},
	}
	for _, tc := range cases {
		if got := stationViewAssignmentScore(tc.assignment, project, tc.edge); got != tc.want {
			t.Fatalf("%s score got=%d want=%d", tc.name, got, tc.want)
		}
	}
}

func newRepositoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func intPtr(value int) *int {
	return &value
}

func boolTestPtr(value bool) *bool {
	return &value
}
