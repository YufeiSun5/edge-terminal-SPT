package query

import "time"

type GatewayConfig struct {
	ID               int       `gorm:"column:id;primaryKey" json:"id"`
	EdgeInstanceID   string    `gorm:"column:edge_instance_id" json:"edge_instance_id"`
	Name             string    `gorm:"column:name" json:"name"`
	Broker           string    `gorm:"column:broker" json:"broker"`
	ClientID         string    `gorm:"column:client_id" json:"client_id"`
	Username         string    `gorm:"column:username" json:"username"`
	Password         string    `gorm:"column:password" json:"-"`
	Topic            string    `gorm:"column:topic" json:"topic"`
	QOS              byte      `gorm:"column:qos" json:"qos"`
	ParserType       string    `gorm:"column:parser_type" json:"parser_type"`
	KIOClientID      string    `gorm:"column:kio_client_id" json:"kio_client_id"`
	KIOWriter        string    `gorm:"column:kio_writer" json:"kio_writer"`
	KIOWriteUsername string    `gorm:"column:kio_write_username" json:"kio_write_username"`
	KIOWritePassword string    `gorm:"column:kio_write_password" json:"-"`
	SetDataTopic     string    `gorm:"column:setdata_topic" json:"setdata_topic"`
	WriteResultTopic string    `gorm:"column:write_result_topic" json:"write_result_topic"`
	QueryAllTopic    string    `gorm:"column:query_all_topic" json:"query_all_topic"`
	Enabled          bool      `gorm:"column:enabled" json:"enabled"`
	CreatedAt        time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt        time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (GatewayConfig) TableName() string { return "sys_gateways" }

func (q *StationViewQuery) ListGatewayConfigs() ([]GatewayConfig, error) {
	var gateways []GatewayConfig
	err := q.db.Order("id ASC").Find(&gateways).Error
	if gateways == nil {
		gateways = []GatewayConfig{}
	}
	return gateways, err
}

func (q *StationViewQuery) GetGatewayConfig(id int) (GatewayConfig, error) {
	var gateway GatewayConfig
	err := q.db.First(&gateway, "id = ?", id).Error
	return gateway, err
}
