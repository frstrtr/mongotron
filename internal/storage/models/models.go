package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Address represents a monitored blockchain address
type Address struct {
	ID             primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Address        string             `bson:"address" json:"address"`
	Network        string             `bson:"network" json:"network"` // "tron", "tron-nile", etc.
	Balance        int64              `bson:"balance" json:"balance"`
	Type           string             `bson:"type" json:"type"` // "account", "contract"
	FirstSeen      time.Time          `bson:"first_seen" json:"firstSeen"`
	LastActivity   time.Time          `bson:"last_activity" json:"lastActivity"`
	TxCount        int64              `bson:"tx_count" json:"txCount"`
	SubscriptionID string             `bson:"subscription_id,omitempty" json:"subscriptionId,omitempty"`
	Metadata       map[string]interface{} `bson:"metadata,omitempty" json:"metadata,omitempty"`
	CreatedAt      time.Time          `bson:"created_at" json:"createdAt"`
	UpdatedAt      time.Time          `bson:"updated_at" json:"updatedAt"`
}

// Transaction represents a blockchain transaction
type Transaction struct {
	ID              primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	TxHash          string             `bson:"tx_hash" json:"txHash"`
	TxID            string             `bson:"tx_id" json:"txId"`
	Network         string             `bson:"network" json:"network"`
	BlockNumber     int64              `bson:"block_number" json:"blockNumber"`
	BlockHash       string             `bson:"block_hash" json:"blockHash"`
	BlockTimestamp  int64              `bson:"block_timestamp" json:"blockTimestamp"`
	FromAddress     string             `bson:"from_address" json:"fromAddress"`
	ToAddress       string             `bson:"to_address" json:"toAddress"`
	Amount          int64              `bson:"amount" json:"amount"`
	AssetName       string             `bson:"asset_name,omitempty" json:"assetName,omitempty"`
	ContractType    string             `bson:"contract_type" json:"contractType"`
	Success         bool               `bson:"success" json:"success"`
	EnergyUsage     int64              `bson:"energy_usage,omitempty" json:"energyUsage,omitempty"`
	EnergyFee       int64              `bson:"energy_fee,omitempty" json:"energyFee,omitempty"`
	NetUsage        int64              `bson:"net_usage,omitempty" json:"netUsage,omitempty"`
	NetFee          int64              `bson:"net_fee,omitempty" json:"netFee,omitempty"`
	Fee             int64              `bson:"fee" json:"fee"`
	RawData         map[string]interface{} `bson:"raw_data,omitempty" json:"rawData,omitempty"`
	CreatedAt       time.Time          `bson:"created_at" json:"createdAt"`
	UpdatedAt       time.Time          `bson:"updated_at" json:"updatedAt"`
}

// Event represents a blockchain event
type Event struct {
	ID             primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	EventID        string             `bson:"event_id" json:"eventId"`
	Network        string             `bson:"network" json:"network"`
	Type           string             `bson:"type" json:"type"` // "Transfer", "Approval", "ContractEvent", etc.
	Address        string             `bson:"address" json:"address"` // Contract or account address
	TxHash         string             `bson:"tx_hash" json:"txHash"`
	BlockNumber    int64              `bson:"block_number" json:"blockNumber"`
	BlockTimestamp int64              `bson:"block_timestamp" json:"blockTimestamp"`
	Data           map[string]interface{} `bson:"data" json:"data"`
	Topics         []string           `bson:"topics,omitempty" json:"topics,omitempty"`
	SubscriptionID string             `bson:"subscription_id,omitempty" json:"subscriptionId,omitempty"`
	Processed      bool               `bson:"processed" json:"processed"`
	CreatedAt      time.Time          `bson:"created_at" json:"createdAt"`
	ExpiresAt      time.Time          `bson:"expires_at,omitempty" json:"expiresAt,omitempty"` // TTL index
}

// Webhook represents a webhook configuration
type Webhook struct {
	ID             primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	URL            string             `bson:"url" json:"url"`
	SubscriptionID string             `bson:"subscription_id" json:"subscriptionId"`
	Events         []string           `bson:"events" json:"events"` // Event types to trigger webhook
	Active         bool               `bson:"active" json:"active"`
	RetryCount     int                `bson:"retry_count" json:"retryCount"`
	MaxRetries     int                `bson:"max_retries" json:"maxRetries"`
	NextRetry      *time.Time         `bson:"next_retry,omitempty" json:"nextRetry,omitempty"`
	Status         string             `bson:"status" json:"status"` // "active", "failing", "disabled"
	LastSuccess    *time.Time         `bson:"last_success,omitempty" json:"lastSuccess,omitempty"`
	LastError      string             `bson:"last_error,omitempty" json:"lastError,omitempty"`
	CreatedAt      time.Time          `bson:"created_at" json:"createdAt"`
	UpdatedAt      time.Time          `bson:"updated_at" json:"updatedAt"`
}
