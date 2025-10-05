package client

import (
	"context"
	"fmt"
	"time"

	"github.com/fbsobreira/gotron-sdk/pkg/proto/api"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/frstrtr/mongotron/pkg/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TronClient handles connection to Tron blockchain node
type TronClient struct {
	conn           *grpc.ClientConn
	walletClient   api.WalletClient
	walletSolidity api.WalletSolidityClient
	host           string
	port           int
	logger         *logger.Logger
}

// Config holds Tron client configuration
type Config struct {
	Host           string
	Port           int
	Timeout        time.Duration
	MaxRetries     int
	BackoffInterval time.Duration
	KeepAlive      time.Duration
}

// NewTronClient creates a new Tron blockchain client
func NewTronClient(cfg Config, log *logger.Logger) (*TronClient, error) {
	if log == nil {
		defaultLog := logger.NewDefault()
		log = &defaultLog
	}

	client := &TronClient{
		host:   cfg.Host,
		port:   cfg.Port,
		logger: log,
	}

	if err := client.connect(cfg); err != nil {
		return nil, fmt.Errorf("failed to connect to Tron node: %w", err)
	}

	return client, nil
}

// connect establishes connection to Tron node
func (c *TronClient) connect(cfg Config) error {
	address := fmt.Sprintf("%s:%d", c.host, c.port)
	
	c.logger.Info().
		Str("address", address).
		Msg("Connecting to Tron node")

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	}

	conn, err := grpc.DialContext(ctx, address, opts...)
	if err != nil {
		return fmt.Errorf("failed to dial: %w", err)
	}

	c.conn = conn
	c.walletClient = api.NewWalletClient(conn)
	c.walletSolidity = api.NewWalletSolidityClient(conn)

	c.logger.Info().
		Str("address", address).
		Msg("Successfully connected to Tron node")

	return nil
}

// GetNowBlock retrieves the latest block from the blockchain
func (c *TronClient) GetNowBlock(ctx context.Context) (*core.Block, error) {
	req := &api.EmptyMessage{}
	
	blockExt, err := c.walletClient.GetNowBlock2(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get current block: %w", err)
	}

	// Extract the Block from BlockExtention
	if blockExt == nil || blockExt.GetBlockHeader() == nil {
		return nil, fmt.Errorf("received nil block extension")
	}

	// Construct Block from BlockExtention
	block := &core.Block{
		BlockHeader:  blockExt.GetBlockHeader(),
		Transactions: extractTransactionsFromExt(blockExt.GetTransactions()),
	}

	return block, nil
}

// GetBlockByNum retrieves a specific block by number
func (c *TronClient) GetBlockByNum(ctx context.Context, blockNum int64) (*core.Block, error) {
	req := &api.NumberMessage{
		Num: blockNum,
	}
	
	blockExt, err := c.walletClient.GetBlockByNum2(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get block %d: %w", blockNum, err)
	}

	if blockExt == nil || blockExt.GetBlockHeader() == nil {
		return nil, fmt.Errorf("received nil block extension for block %d", blockNum)
	}

	// Construct Block from BlockExtention
	block := &core.Block{
		BlockHeader:  blockExt.GetBlockHeader(),
		Transactions: extractTransactionsFromExt(blockExt.GetTransactions()),
	}

	return block, nil
}

// extractTransactionsFromExt extracts transactions from TransactionExtention list
func extractTransactionsFromExt(txExts []*api.TransactionExtention) []*core.Transaction {
	txs := make([]*core.Transaction, 0, len(txExts))
	for _, txExt := range txExts {
		if txExt != nil && txExt.Transaction != nil {
			txs = append(txs, txExt.Transaction)
		}
	}
	return txs
}

// GetTransactionInfoById retrieves transaction details including events and logs
func (c *TronClient) GetTransactionInfoById(ctx context.Context, txID string) (*core.TransactionInfo, error) {
	req := &api.BytesMessage{
		Value: []byte(txID),
	}
	
	txInfo, err := c.walletClient.GetTransactionInfoById(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction info for %s: %w", txID, err)
	}

	return txInfo, nil
}

// GetTransactionById retrieves transaction by ID
func (c *TronClient) GetTransactionById(ctx context.Context, txID string) (*core.Transaction, error) {
	req := &api.BytesMessage{
		Value: []byte(txID),
	}
	
	tx, err := c.walletClient.GetTransactionById(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction %s: %w", txID, err)
	}

	return tx, nil
}

// GetAccount retrieves account information
func (c *TronClient) GetAccount(ctx context.Context, address []byte) (*core.Account, error) {
	req := &core.Account{
		Address: address,
	}
	
	account, err := c.walletClient.GetAccount(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	return account, nil
}

// GetAccountResource retrieves account resource information (bandwidth, energy)
func (c *TronClient) GetAccountResource(ctx context.Context, address []byte) (*api.AccountResourceMessage, error) {
	req := &core.Account{
		Address: address,
	}
	
	resource, err := c.walletClient.GetAccountResource(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get account resource: %w", err)
	}

	return resource, nil
}

// GetContract retrieves smart contract information including ABI
func (c *TronClient) GetContract(ctx context.Context, contractAddress []byte) (*core.SmartContract, error) {
	req := &api.BytesMessage{
		Value: contractAddress,
	}
	
	contract, err := c.walletClient.GetContract(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get contract: %w", err)
	}

	return contract, nil
}

// GetChainParameters retrieves blockchain parameters
func (c *TronClient) GetChainParameters(ctx context.Context) (*core.ChainParameters, error) {
	req := &api.EmptyMessage{}
	
	params, err := c.walletClient.GetChainParameters(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get chain parameters: %w", err)
	}

	return params, nil
}

// GetNodeInfo retrieves information about the connected node
func (c *TronClient) GetNodeInfo(ctx context.Context) (*core.NodeInfo, error) {
	req := &api.EmptyMessage{}
	
	info, err := c.walletClient.GetNodeInfo(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get node info: %w", err)
	}

	return info, nil
}

// GetBlockByLatestNum retrieves the latest N blocks
func (c *TronClient) GetBlockByLatestNum(ctx context.Context, num int64) (*api.BlockList, error) {
	req := &api.NumberMessage{
		Num: num,
	}
	
	blocks, err := c.walletClient.GetBlockByLatestNum(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest %d blocks: %w", num, err)
	}

	return blocks, nil
}

// IsConnected checks if the client is connected to the node
func (c *TronClient) IsConnected() bool {
	if c.conn == nil {
		return false
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	_, err := c.GetNodeInfo(ctx)
	return err == nil
}

// Close closes the connection to the Tron node
func (c *TronClient) Close() error {
	if c.conn != nil {
		c.logger.Info().Msg("Closing connection to Tron node")
		return c.conn.Close()
	}
	return nil
}

// Reconnect attempts to reconnect to the Tron node
func (c *TronClient) Reconnect(cfg Config) error {
	c.logger.Warn().Msg("Attempting to reconnect to Tron node")
	
	if err := c.Close(); err != nil {
		c.logger.Error().Err(err).Msg("Error closing old connection")
	}
	
	return c.connect(cfg)
}
