package database

import (
	"errors"
	"time"

	"spindle-edge/backend/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *Repository) CountUsers() (int64, error) {
	var count int64
	err := r.db.Model(&models.SysUser{}).Count(&count).Error
	return count, err
}

func (r *Repository) CreateUser(user *models.SysUser) error {
	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now
	if user.PermissionsVersion == 0 {
		user.PermissionsVersion = 1
	}
	return r.db.Create(user).Error
}

func (r *Repository) FindUserByUsername(username string) (models.SysUser, error) {
	var user models.SysUser
	err := r.db.First(&user, "username = ?", username).Error
	return user, err
}

func (r *Repository) FindUserByID(id uint) (models.SysUser, error) {
	var user models.SysUser
	err := r.db.First(&user, "id = ?", id).Error
	return user, err
}

func (r *Repository) ListUsers() ([]models.SysUser, error) {
	var users []models.SysUser
	err := r.db.Order("id asc").Find(&users).Error
	return users, err
}

func (r *Repository) UpdateUser(id uint, updates map[string]interface{}) (models.SysUser, error) {
	if len(updates) == 0 {
		return r.FindUserByID(id)
	}
	updates["updated_at"] = time.Now()
	if _, ok := updates["role"]; ok {
		updates["permissions_version"] = gorm.Expr("permissions_version + 1")
	}
	if _, ok := updates["enabled"]; ok {
		updates["permissions_version"] = gorm.Expr("permissions_version + 1")
	}
	if _, ok := updates["password_hash"]; ok {
		updates["permissions_version"] = gorm.Expr("permissions_version + 1")
	}
	if err := r.db.Model(&models.SysUser{}).
		Where("id = ?", id).
		Updates(updates).Error; err != nil {
		return models.SysUser{}, err
	}
	return r.FindUserByID(id)
}

func (r *Repository) DeleteUser(id uint) error {
	return r.db.Delete(&models.SysUser{}, "id = ?", id).Error
}

func (r *Repository) UpdateUserLastLogin(id uint, at time.Time) error {
	return r.db.Model(&models.SysUser{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"last_login_at": at,
			"updated_at":    at,
		}).Error
}

func (r *Repository) UpsertServiceClient(client models.SysServiceClient) error {
	now := time.Now()
	client.CreatedAt = now
	client.UpdatedAt = now
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "client_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"secret_hash":   client.SecretHash,
			"scopes":        client.Scopes,
			"allowed_cidrs": client.AllowedCIDRs,
			"enabled":       client.Enabled,
			"expires_at":    client.ExpiresAt,
			"updated_at":    now,
		}),
	}).Create(&client).Error
}

func (r *Repository) FindServiceClientBySecretHash(secretHash string) (models.SysServiceClient, error) {
	var client models.SysServiceClient
	err := r.db.First(&client, "secret_hash = ?", secretHash).Error
	return client, err
}

func (r *Repository) UpdateServiceClientLastUsed(id uint, at time.Time) error {
	return r.db.Model(&models.SysServiceClient{}).
		Where("id = ?", id).
		Update("last_used_at", at).Error
}

func (r *Repository) CreateSSOTicket(ticket *models.SysSSOTicket) error {
	now := time.Now()
	ticket.CreatedAt = now
	return r.db.Create(ticket).Error
}

var (
	ErrSSOTicketInvalid = errors.New("sso ticket is invalid")
	ErrSSOTicketExpired = errors.New("sso ticket is expired")
	ErrSSOTicketUsed    = errors.New("sso ticket is already used")
)

func (r *Repository) ConsumeSSOTicket(ticketHash string, edgeInstanceID string, now time.Time) (models.SysUser, error) {
	var user models.SysUser
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var ticket models.SysSSOTicket
		if err := tx.First(&ticket, "ticket_hash = ? AND edge_instance_id = ?", ticketHash, edgeInstanceID).Error; err != nil {
			return ErrSSOTicketInvalid
		}
		if ticket.UsedAt != nil {
			return ErrSSOTicketUsed
		}
		if !ticket.ExpiresAt.After(now) {
			return ErrSSOTicketExpired
		}
		if err := tx.First(&user, "id = ?", ticket.UserID).Error; err != nil {
			return err
		}
		if !user.Enabled || user.Role != ticket.Role || user.PermissionsVersion != ticket.PermissionsVersion {
			return ErrSSOTicketInvalid
		}
		return tx.Model(&models.SysSSOTicket{}).
			Where("id = ? AND used_at IS NULL", ticket.ID).
			Update("used_at", now).Error
	})
	return user, err
}

func (r *Repository) CreateAuditLog(entry *models.SysAuditLog) error {
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	if entry.Detail == "" {
		entry.Detail = "{}"
	}
	return r.db.Create(entry).Error
}

func (r *Repository) CreateEdgeControlCommand(command *models.EdgeControlCommand) error {
	now := time.Now()
	command.CreatedAt = now
	command.UpdatedAt = now
	if command.ReceivedAt.IsZero() {
		command.ReceivedAt = now
	}
	if command.Status == "" {
		command.Status = "received"
	}
	if command.RequestJSON == "" {
		command.RequestJSON = "{}"
	}
	if command.ResultJSON == "" {
		command.ResultJSON = "{}"
	}
	return r.db.Create(command).Error
}

func (r *Repository) FindEdgeControlCommand(clientID string, commandID string) (models.EdgeControlCommand, error) {
	var command models.EdgeControlCommand
	err := r.db.First(&command, "client_id = ? AND command_id = ?", clientID, commandID).Error
	return command, err
}

func (r *Repository) MarkEdgeControlCommandRunning(id uint64) error {
	return r.db.Model(&models.EdgeControlCommand{}).
		Where("id = ? AND status = ?", id, "received").
		Updates(map[string]interface{}{
			"status":     "running",
			"updated_at": time.Now(),
		}).Error
}

func (r *Repository) UpdateEdgeControlCommandResult(id uint64, status string, targetID string, resultJSON string, errorCode string, errorMessage string) error {
	if resultJSON == "" {
		resultJSON = "{}"
	}
	updates := map[string]interface{}{
		"result_json":   resultJSON,
		"error_code":    errorCode,
		"error_message": errorMessage,
		"updated_at":    time.Now(),
	}
	if status != "" {
		updates["status"] = status
	}
	if targetID != "" {
		updates["target_id"] = targetID
	}
	return r.db.Model(&models.EdgeControlCommand{}).Where("id = ?", id).Updates(updates).Error
}

func (r *Repository) CompleteEdgeControlCommand(id uint64, status string, targetID string, resultJSON string, errorCode string, errorMessage string) error {
	now := time.Now()
	if resultJSON == "" {
		resultJSON = "{}"
	}
	updates := map[string]interface{}{
		"status":        status,
		"result_json":   resultJSON,
		"error_code":    errorCode,
		"error_message": errorMessage,
		"completed_at":  &now,
		"updated_at":    now,
	}
	if targetID != "" {
		updates["target_id"] = targetID
	}
	return r.db.Model(&models.EdgeControlCommand{}).Where("id = ?", id).Updates(updates).Error
}

type AuditLogListFilter struct {
	ActorType  string
	ActorID    string
	Action     string
	TargetType string
	TargetID   string
	Result     string
	From       *time.Time
	To         *time.Time
	Limit      int
	Offset     int
}

func (r *Repository) ListAuditLogs(filter AuditLogListFilter) ([]models.SysAuditLog, int64, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	query := r.db.Model(&models.SysAuditLog{})
	if filter.ActorType != "" {
		query = query.Where("actor_type = ?", filter.ActorType)
	}
	if filter.ActorID != "" {
		query = query.Where("actor_id = ?", filter.ActorID)
	}
	if filter.Action != "" {
		query = query.Where("action = ?", filter.Action)
	}
	if filter.TargetType != "" {
		query = query.Where("target_type = ?", filter.TargetType)
	}
	if filter.TargetID != "" {
		query = query.Where("target_id = ?", filter.TargetID)
	}
	if filter.Result != "" {
		query = query.Where("result = ?", filter.Result)
	}
	if filter.From != nil {
		query = query.Where("created_at >= ?", *filter.From)
	}
	if filter.To != nil {
		query = query.Where("created_at <= ?", *filter.To)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []models.SysAuditLog
	err := query.Order("created_at desc, id desc").Limit(limit).Offset(offset).Find(&items).Error
	return items, total, err
}
