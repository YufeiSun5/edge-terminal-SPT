package models

import "time"

type GatewayConfig struct {
	ID               int       `gorm:"column:id;primaryKey" json:"id"`
	Name             string    `gorm:"column:name;size:128;not null" json:"name"`
	Broker           string    `gorm:"column:broker;size:255;not null" json:"broker"`
	ClientID         string    `gorm:"column:client_id;size:128;not null" json:"client_id"`
	Username         string    `gorm:"column:username;size:128" json:"username"`
	Password         string    `gorm:"column:password;size:255" json:"-"`
	Topic            string    `gorm:"column:topic;size:255;not null" json:"topic"`
	QOS              byte      `gorm:"column:qos;default:1" json:"qos"`
	ParserType       string    `gorm:"column:parser_type;size:64;default:kingiot_kio" json:"parser_type"`
	KIOClientID      string    `gorm:"column:kio_client_id;size:128" json:"kio_client_id"`
	KIOWriter        string    `gorm:"column:kio_writer;size:128" json:"kio_writer"`
	KIOWriteUsername string    `gorm:"column:kio_write_username;size:128" json:"kio_write_username"`
	KIOWritePassword string    `gorm:"column:kio_write_password;size:255" json:"-"`
	SetDataTopic     string    `gorm:"column:setdata_topic;size:255" json:"setdata_topic"`
	WriteResultTopic string    `gorm:"column:write_result_topic;size:255" json:"write_result_topic"`
	QueryAllTopic    string    `gorm:"column:query_all_topic;size:255" json:"query_all_topic"`
	Enabled          bool      `gorm:"column:enabled;default:true" json:"enabled"`
	CreatedAt        time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt        time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (GatewayConfig) TableName() string {
	return "sys_gateways"
}
