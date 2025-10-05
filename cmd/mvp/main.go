package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/fbsobreira/gotron-sdk/pkg/address"
	"github.com/fbsobreira/gotron-sdk/pkg/common"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/frstrtr/mongotron/internal/blockchain/client"
	"github.com/frstrtr/mongotron/internal/blockchain/contract"
	"github.com/frstrtr/mongotron/internal/blockchain/monitor"
	"github.com/frstrtr/mongotron/internal/storage"
	"github.com/frstrtr/mongotron/internal/storage/models"
	"github.com/frstrtr/mongotron/pkg/logger"
	"google.golang.org/protobuf/types/known/anypb"
)

const (
	defaultNetwork     = "tron-nile"
	defaultPollInterval = 3 * time.Second
)

var (
	abiDecoder *contract.ABIDecoder
	tronClient *client.TronClient
)

// hexToBase58Address converts a hex address to Base58 Tron address format
func hexToBase58Address(hexAddr string) string {
	if hexAddr == "" {
		return ""
	}
	// Remove 0x prefix if present
	hexAddr = strings.TrimPrefix(hexAddr, "0x")
	
	// Special case for contract creation
	if hexAddr == "CreateSmartContract" {
		return "CreateSmartContract"
	}
	
	// Convert to address and then to Base58
	addr := address.HexToAddress(hexAddr)
	base58 := common.EncodeCheck(addr.Bytes())
	return base58
}

// getTransactionTypeDisplay returns a human-readable transaction type
func getTransactionTypeDisplay(contractType string) string {
	typeMap := map[string]string{
		// Account Operations
		"AccountCreateContract":           "Create Account",
		"UpdateAccountContract":           "Update Account",
		"SetAccountIdContract":            "Set Account ID",
		"AccountPermissionUpdateContract": "Update Permissions",
		
		// TRX Transfers
		"TransferContract": "Transfer (TRX)",
		
		// TRC10 Token Operations
		"TransferAssetContract":          "Transfer (TRC10)",
		"AssetIssueContract":             "Issue Token (TRC10)",
		"UpdateAssetContract":            "Update Token (TRC10)",
		"UnfreezeAssetContract":          "Unfreeze Token (TRC10)",
		"ParticipateAssetIssueContract":  "Participate Token Sale",
		
		// Smart Contract Operations
		"TriggerSmartContract":    "Smart Contract",
		"CreateSmartContract":     "Create Contract",
		"UpdateEnergyLimitContract": "Update Energy Limit",
		"ClearABIContract":          "Clear Contract ABI",
		"UpdateSettingContract":     "Update Contract Setting",
		"UpdateBrokerageContract":   "Update Brokerage",
		
		// Resource Management (Staking v1)
		"FreezeBalanceContract":   "Freeze Balance",
		"UnfreezeBalanceContract": "Unfreeze Balance",
		
		// Resource Management (Staking v2)
		"FreezeBalanceV2Contract":        "Stake (v2)",
		"UnfreezeBalanceV2Contract":      "Unstake (v2)",
		"DelegateResourceContract":       "Delegate Resource",
		"UnDelegateResourceContract":     "Undelegate Resource",
		"WithdrawExpireUnfreezeContract": "Withdraw Unstaked",
		"CancelAllUnfreezeV2Contract":    "Cancel All Unstake",
		
		// Witness Operations
		"WitnessCreateContract": "Create Witness",
		"WitnessUpdateContract": "Update Witness",
		"VoteWitnessContract":   "Vote Witness",
		"WithdrawBalanceContract": "Withdraw Rewards",
		
		// Proposal Operations
		"ProposalCreateContract":  "Create Proposal",
		"ProposalApproveContract": "Approve Proposal",
		"ProposalDeleteContract":  "Delete Proposal",
		
		// Exchange Operations (DEX)
		"ExchangeCreateContract":      "Create Exchange",
		"ExchangeInjectContract":      "Inject Exchange",
		"ExchangeWithdrawContract":    "Withdraw Exchange",
		"ExchangeTransactionContract": "Exchange Trade",
		
		// Shield (Privacy) Operations
		"ShieldedTransferContract": "Shielded Transfer",
		
		// Market Operations
		"MarketSellAssetContract":   "Market Sell",
		"MarketCancelOrderContract": "Market Cancel Order",
	}
	
	if display, ok := typeMap[contractType]; ok {
		return display
	}
	
	// If unknown, return the raw type
	return contractType
}

// extractSmartContractData extracts the data field from TriggerSmartContract parameter
func extractSmartContractData(contractData map[string]interface{}) []byte {
	// The parameter field is a *anypb.Any protobuf message
	if parameterAny, ok := contractData["parameter"].(*anypb.Any); ok {
		// Unmarshal the Any type value
		var trigger core.TriggerSmartContract
		if err := parameterAny.UnmarshalTo(&trigger); err == nil {
			return trigger.GetData()
		}
	}
	return nil
}

// getSmartContractInteraction decodes smart contract interaction and returns human-readable type
func getSmartContractInteraction(contractAddressHex string, contractData map[string]interface{}, verbose bool, logger *logger.Logger) string {
	if abiDecoder == nil {
		return "Smart Contract"
	}

	// Extract the data field from TriggerSmartContract
	callData := extractSmartContractData(contractData)
	if len(callData) < 4 {
		return "Smart Contract"
	}

	// Convert hex address to base58
	contractAddress := hexToBase58Address(contractAddressHex)
	
	// Try to load ABI if not cached
	if !abiDecoder.HasABI(contractAddress) && tronClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		// Decode hex address
		addressBytes, err := hex.DecodeString(strings.TrimPrefix(contractAddressHex, "41"))
		if err == nil {
			// Prepend 0x41 for Tron address
			fullAddress := append([]byte{0x41}, addressBytes...)
			
			// Fetch contract info
			smartContract, err := tronClient.GetContract(ctx, fullAddress)
			if err == nil && smartContract != nil && smartContract.GetAbi() != nil {
				abiJSON := smartContract.GetAbi().String()
				if abiJSON != "" {
					// Try to load ABI
					err = abiDecoder.LoadABI(contractAddress, abiJSON)
					if err != nil && verbose && logger != nil {
						logger.Debug().
							Err(err).
							Str("contract", contractAddress).
							Msg("Failed to load ABI")
					} else if verbose && logger != nil {
						logger.Info().
							Str("contract", contractAddress).
							Msg("Contract ABI loaded successfully")
					}
				}
			}
		}
	}

	// Decode method call
	decoded, err := abiDecoder.DecodeMethodCall(contractAddress, callData)
	if err == nil && decoded != nil {
		humanReadable := contract.GetHumanReadableType(decoded.Name)
		if verbose && logger != nil {
			logger.Debug().
				Str("contract", contractAddress).
				Str("method", decoded.Name).
				Str("signature", decoded.Signature).
				Str("humanReadable", humanReadable).
				Msg("Decoded smart contract interaction")
		}
		return humanReadable
	}

	return "Smart Contract"
}

func main() {
	// Parse command line flags
	var (
		tronHost     = flag.String("tron-host", "nileVM.lan", "Tron node host")
		tronPort     = flag.Int("tron-port", 50051, "Tron node gRPC port")
		mongoURI     = flag.String("mongo-uri", "mongodb://mongotron:MongoTron2025@nileVM.lan:27017/mongotron", "MongoDB URI")
		mongoDb      = flag.String("mongo-db", "mongotron", "MongoDB database name")
		watchAddress = flag.String("address", "", "Specific address to watch (leave empty with --monitor)")
		startBlock   = flag.Int64("start-block", 0, "Block number to start from (0 = current)")
		monitorMode  = flag.Bool("monitor", false, "Monitor all addresses in blocks (comprehensive mode)")
		verboseMode  = flag.Bool("verbose", false, "Display detailed information about parsed and stored data")
	)
	flag.Parse()

	// Validate flags
	if !*monitorMode && *watchAddress == "" {
		fmt.Println("Error: Either -address or -monitor flag is required")
		fmt.Println("\nExamples:")
		fmt.Println("  Watch specific address: ./mongotron-mvp -address=TYsbWxNnyTgsZaTFaue9hqpxkU3Fkco94a")
		fmt.Println("  Monitor all addresses:  ./mongotron-mvp -monitor")
		flag.Usage()
		os.Exit(1)
	}

	// Initialize logger
	logInstance := logger.NewDefault()
	log := &logInstance

	if *monitorMode {
		log.Info().
			Str("version", "0.1.0-mvp").
			Str("mode", "comprehensive-monitor").
			Str("tronHost", *tronHost).
			Int("tronPort", *tronPort).
			Msg("Starting MongoTron MVP in Monitor Mode")
	} else {
		log.Info().
			Str("version", "0.1.0-mvp").
			Str("mode", "single-address").
			Str("address", *watchAddress).
			Str("tronHost", *tronHost).
			Int("tronPort", *tronPort).
			Msg("Starting MongoTron MVP")
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize MongoDB connection
	log.Info().Msg("Connecting to MongoDB")
	db, err := storage.NewDatabase(storage.Config{
		URI:            *mongoURI,
		Database:       *mongoDb,
		MaxPoolSize:    100,
		MinPoolSize:    10,
		MaxIdleTime:    5 * time.Minute,
		ConnectTimeout: 10 * time.Second,
	}, log)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to MongoDB")
	}
	defer func() {
		if err := db.Close(ctx); err != nil {
			log.Error().Err(err).Msg("Error closing MongoDB connection")
		}
	}()

	// Initialize database indexes
	log.Info().Msg("Initializing database indexes")
	if err := db.InitializeIndexes(ctx); err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize database indexes")
	}

	// Initialize Tron client
	log.Info().Msg("Connecting to Tron node")
	tronClient, err = client.NewTronClient(client.Config{
		Host:            *tronHost,
		Port:            *tronPort,
		Timeout:         30 * time.Second,
		MaxRetries:      3,
		BackoffInterval: 5 * time.Second,
		KeepAlive:       60 * time.Second,
	}, log)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to Tron node")
	}
	defer func() {
		if err := tronClient.Close(); err != nil {
			log.Error().Err(err).Msg("Error closing Tron client connection")
		}
	}()

	// Initialize ABI decoder
	log.Info().Msg("Initializing smart contract ABI decoder")
	abiDecoder = contract.NewABIDecoder()
	log.Info().Msg("ABI decoder initialized")
	defer func() {
		if err := tronClient.Close(); err != nil {
			log.Error().Err(err).Msg("Error closing Tron client")
		}
	}()

	// Get node info
	nodeInfo, err := tronClient.GetNodeInfo(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to get node info")
	} else {
		log.Info().
			Str("version", nodeInfo.GetConfigNodeInfo().GetCodeVersion()).
			Msg("Connected to Tron node")
	}

	// Start appropriate monitor based on mode
	if *monitorMode {
		// Comprehensive monitoring mode - all addresses
		runComprehensiveMonitor(ctx, tronClient, db, log, *startBlock, *verboseMode)
	} else {
		// Single address monitoring mode
		runSingleAddressMonitor(ctx, tronClient, db, log, *watchAddress, *startBlock, *verboseMode)
	}
}

// runSingleAddressMonitor runs the original single-address monitoring
func runSingleAddressMonitor(ctx context.Context, tronClient *client.TronClient, db *storage.Database, log *logger.Logger, watchAddress string, startBlock int64, verbose bool) {
	// Register the address in the database
	log.Info().Msg("Registering watched address in database")
	existingAddr, err := db.AddressRepo.FindByAddress(ctx, watchAddress)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to check existing address")
	}

	if existingAddr == nil {
		addr := &models.Address{
			Address:      watchAddress,
			Network:      defaultNetwork,
			Type:         "account",
			FirstSeen:    time.Now(),
			LastActivity: time.Now(),
			TxCount:      0,
			Metadata:     make(map[string]interface{}),
		}
		if err := db.AddressRepo.Create(ctx, addr); err != nil {
			log.Fatal().Err(err).Msg("Failed to create address record")
		}
		log.Info().Str("address", watchAddress).Msg("Address registered in database")
	} else {
		log.Info().Str("address", watchAddress).Msg("Address already exists in database")
	}

	// Initialize address monitor
	log.Info().Msg("Initializing address monitor")
	addrMonitor, err := monitor.NewAddressMonitor(
		tronClient,
		monitor.Config{
			WatchAddress: watchAddress,
			PollInterval: defaultPollInterval,
			StartBlock:   startBlock,
		},
		log,
	)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create address monitor")
	}

	// Start monitoring
	if err := addrMonitor.Start(); err != nil {
		log.Fatal().Err(err).Msg("Failed to start address monitor")
	}

	log.Info().
		Str("address", watchAddress).
		Int64("startBlock", startBlock).
		Msg("MongoTron MVP started successfully - watching for events")

	// Process events from monitor
	go processEvents(ctx, addrMonitor, db, log, verbose)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Print status every 30 seconds
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-sigChan:
			log.Info().Msg("Shutdown signal received")
			addrMonitor.Stop()
			time.Sleep(2 * time.Second)
			log.Info().Msg("MongoTron MVP stopped")
			return

		case <-ticker.C:
			lastBlock := addrMonitor.GetLastBlockNumber()
			txCount, _ := db.TransactionRepo.CountByAddress(ctx, watchAddress)
			eventCount, _ := db.EventRepo.Count(ctx)

			log.Info().
				Int64("lastBlock", lastBlock).
				Int64("txCount", txCount).
				Int64("eventCount", eventCount).
				Msg("Status update")
		}
	}
}

// runComprehensiveMonitor runs comprehensive block monitoring for all addresses
func runComprehensiveMonitor(ctx context.Context, tronClient *client.TronClient, db *storage.Database, log *logger.Logger, startBlock int64, verbose bool) {
	// Initialize block monitor
	log.Info().Msg("Initializing comprehensive block monitor")
	blockMonitor, err := monitor.NewBlockMonitor(
		tronClient,
		monitor.BlockMonitorConfig{
			PollInterval: defaultPollInterval,
			StartBlock:   startBlock,
		},
		log,
	)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create block monitor")
	}

	// Start monitoring
	if err := blockMonitor.Start(); err != nil {
		log.Fatal().Err(err).Msg("Failed to start block monitor")
	}

	log.Info().
		Int64("startBlock", startBlock).
		Msg("MongoTron MVP started successfully - monitoring all addresses")

	// Process block events
	go processBlockEvents(ctx, blockMonitor, db, log, verbose)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Print status every 30 seconds
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-sigChan:
			log.Info().Msg("Shutdown signal received")
			blockMonitor.Stop()
			time.Sleep(2 * time.Second)
			log.Info().Msg("MongoTron MVP stopped")
			return

		case <-ticker.C:
			lastBlock := blockMonitor.GetLastBlockNumber()
			addressCount, _ := db.AddressRepo.Count(ctx)
			txCount, _ := db.TransactionRepo.Count(ctx)
			eventCount, _ := db.EventRepo.Count(ctx)

			log.Info().
				Int64("lastBlock", lastBlock).
				Int64("addresses", addressCount).
				Int64("txCount", txCount).
				Int64("eventCount", eventCount).
				Msg("Status update")
		}
	}
}

// processEvents processes events from the address monitor and stores them in MongoDB
func processEvents(ctx context.Context, mon *monitor.AddressMonitor, db *storage.Database, log *logger.Logger, verbose bool) {
	for event := range mon.Events() {
		if err := storeEvent(ctx, event, db, log, verbose); err != nil {
			log.Error().
				Err(err).
				Str("txHash", event.TransactionHash).
				Msg("Failed to store event")
			continue
		}

		tronTxType := getTransactionTypeDisplay(event.ContractType)

		if verbose {
			logEvent := log.Info().
				Int64("block", event.BlockNumber).
				Str("txHash", event.TransactionHash).
				Str("TronTXType", tronTxType).
				Str("contractType", event.ContractType)
			
			// If it's a smart contract interaction, try to decode it and add SCTXType
			if event.ContractType == "TriggerSmartContract" && event.RawTransaction != nil {
				// Extract contract data from raw transaction
				txRaw := event.RawTransaction.GetRawData()
				if len(txRaw.GetContract()) > 0 {
					contract := txRaw.GetContract()[0]
					contractData := map[string]interface{}{
						"type":      contract.GetType().String(),
						"parameter": contract.GetParameter(),
					}
					scInteractionType := getSmartContractInteraction(event.To, contractData, verbose, log)
					if scInteractionType != "Smart Contract" {
						logEvent = logEvent.Str("SCTXType", scInteractionType)
					}
				}
			}
			
			logEvent.
				Str("from", hexToBase58Address(event.From)).
				Str("fromHex", event.From).
				Str("to", hexToBase58Address(event.To)).
				Str("toHex", event.To).
				Int64("amount", event.Amount).
				Bool("success", event.Success).
				Interface("eventData", event.EventData).
				Msg("Event stored successfully")
		} else {
			logEvent := log.Info().
				Int64("block", event.BlockNumber).
				Str("txHash", event.TransactionHash).
				Str("TronTXType", tronTxType)
			
			// If it's a smart contract interaction, try to decode it and add SCTXType
			if event.ContractType == "TriggerSmartContract" && event.RawTransaction != nil {
				txRaw := event.RawTransaction.GetRawData()
				if len(txRaw.GetContract()) > 0 {
					contract := txRaw.GetContract()[0]
					contractData := map[string]interface{}{
						"type":      contract.GetType().String(),
						"parameter": contract.GetParameter(),
					}
					scInteractionType := getSmartContractInteraction(event.To, contractData, false, log)
					if scInteractionType != "Smart Contract" {
						logEvent = logEvent.Str("SCTXType", scInteractionType)
					}
				}
			}
			
			logEvent.
				Str("from", hexToBase58Address(event.From)).
				Str("to", hexToBase58Address(event.To)).
				Int64("amount", event.Amount).
				Bool("success", event.Success).
				Msg("Event stored successfully")
		}
	}
}

// storeEvent stores an event and its related transaction in the database
func storeEvent(ctx context.Context, event *monitor.AddressEvent, db *storage.Database, log *logger.Logger, verbose bool) error {
	// Check if transaction already exists
	existingTx, err := db.TransactionRepo.FindByHash(ctx, event.TransactionHash)
	if err != nil {
		return fmt.Errorf("failed to check existing transaction: %w", err)
	}

	// Create transaction record if it doesn't exist
	if existingTx == nil {
		tx := &models.Transaction{
			TxHash:         event.TransactionHash,
			TxID:           event.TransactionID,
			Network:        defaultNetwork,
			BlockNumber:    event.BlockNumber,
			BlockHash:      event.BlockHash,
			BlockTimestamp: event.BlockTimestamp,
			FromAddress:    event.From,
			ToAddress:      event.To,
			Amount:         event.Amount,
			AssetName:      event.AssetName,
			ContractType:   event.ContractType,
			Success:        event.Success,
			RawData:        make(map[string]interface{}),
		}

		// Extract resource usage from event data if available
		if event.EventData != nil {
			if energyUsage, ok := event.EventData["energyUsage"].(int64); ok {
				tx.EnergyUsage = energyUsage
			}
			if energyFee, ok := event.EventData["energyFee"].(int64); ok {
				tx.EnergyFee = energyFee
			}
			if netUsage, ok := event.EventData["netUsage"].(int64); ok {
				tx.NetUsage = netUsage
			}
			if netFee, ok := event.EventData["netFee"].(int64); ok {
				tx.NetFee = netFee
			}

			// Store event data in raw_data
			tx.RawData["eventData"] = event.EventData
		}

		if err := db.TransactionRepo.Create(ctx, tx); err != nil {
			return fmt.Errorf("failed to create transaction: %w", err)
		}

		if verbose {
			tronTxType := getTransactionTypeDisplay(event.ContractType)
			
			logEvent := log.Info().
				Str("txHash", event.TransactionHash).
				Str("TronTXType", tronTxType).
				Str("contractType", event.ContractType)
			
			// If it's a smart contract interaction, try to decode it and add SCTXType
			if event.ContractType == "TriggerSmartContract" && event.RawTransaction != nil {
				txRaw := event.RawTransaction.GetRawData()
				if len(txRaw.GetContract()) > 0 {
					contract := txRaw.GetContract()[0]
					contractData := map[string]interface{}{
						"type":      contract.GetType().String(),
						"parameter": contract.GetParameter(),
					}
					scInteractionType := getSmartContractInteraction(event.To, contractData, verbose, log)
					if scInteractionType != "Smart Contract" {
						logEvent = logEvent.Str("SCTXType", scInteractionType)
					}
				}
			}
			
			logEvent.
				Str("from", hexToBase58Address(event.From)).
				Str("fromHex", event.From).
				Str("to", hexToBase58Address(event.To)).
				Str("toHex", event.To).
				Int64("amount", event.Amount).
				Int64("energyUsage", tx.EnergyUsage).
				Int64("netUsage", tx.NetUsage).
				Interface("rawData", tx.RawData).
				Msg("Transaction stored")
		} else {
			log.Debug().
				Str("txHash", event.TransactionHash).
				Msg("Transaction stored")
		}
	}

	// Create event record
	evt := &models.Event{
		EventID:        fmt.Sprintf("%s-%d", event.TransactionHash, event.BlockNumber),
		Network:        defaultNetwork,
		Type:           event.EventType,
		Address:        event.To, // Or could be event.From depending on context
		TxHash:         event.TransactionHash,
		BlockNumber:    event.BlockNumber,
		BlockTimestamp: event.BlockTimestamp,
		Data:           event.EventData,
		Processed:      false,
	}

	if err := db.EventRepo.Create(ctx, evt); err != nil {
		// If event already exists (duplicate), that's okay
		if !isDuplicateKeyError(err) {
			return fmt.Errorf("failed to create event: %w", err)
		}
		log.Debug().Str("eventID", evt.EventID).Msg("Event already exists")
	}

	// Update address activity
	if event.From != "" {
		if err := db.AddressRepo.UpdateActivity(ctx, event.From, time.Unix(event.BlockTimestamp, 0)); err != nil {
			log.Warn().Err(err).Str("address", event.From).Msg("Failed to update address activity")
		}
	}
	if event.To != "" {
		if err := db.AddressRepo.UpdateActivity(ctx, event.To, time.Unix(event.BlockTimestamp, 0)); err != nil {
			log.Warn().Err(err).Str("address", event.To).Msg("Failed to update address activity")
		}
	}

	return nil
}

// processBlockEvents processes comprehensive block events and stores all address data in MongoDB
func processBlockEvents(ctx context.Context, mon *monitor.BlockMonitor, db *storage.Database, log *logger.Logger, verbose bool) {
	for blockEvent := range mon.Events() {
		if err := storeBlockEvent(ctx, blockEvent, db, log, verbose); err != nil {
			log.Error().
				Err(err).
				Int64("block", blockEvent.BlockNumber).
				Msg("Failed to store block event")
			continue
		}

		log.Info().
			Int64("block", blockEvent.BlockNumber).
			Int("txCount", len(blockEvent.Transactions)).
			Int("addresses", len(blockEvent.Addresses)).
			Msg("Block processed and stored")
	}
}

// storeBlockEvent stores comprehensive block data including all addresses and transactions
func storeBlockEvent(ctx context.Context, blockEvent *monitor.BlockEvent, db *storage.Database, log *logger.Logger, verbose bool) error {
	blockTimestamp := time.Unix(blockEvent.BlockTimestamp/1000, 0)

	if verbose {
		log.Info().
			Int64("block", blockEvent.BlockNumber).
			Str("blockHash", blockEvent.BlockHash).
			Time("timestamp", blockTimestamp).
			Int("addressCount", len(blockEvent.Addresses)).
			Int("txCount", len(blockEvent.Transactions)).
			Msg("Processing block data")
		
		// List all transactions in this block
		if len(blockEvent.Transactions) > 0 {
			log.Info().
				Int64("block", blockEvent.BlockNumber).
				Msg("=== Block Transactions ===")
			for i, txData := range blockEvent.Transactions {
				tronTxType := getTransactionTypeDisplay(txData.ContractType)
				
				logEvent := log.Info().
					Int("txIndex", i+1).
					Str("txHash", txData.TxHash).
					Str("TronTXType", tronTxType).
					Str("contractType", txData.ContractType)
				
				// If it's a smart contract interaction, try to decode it and add SCTXType
				if txData.ContractType == "TriggerSmartContract" && len(txData.ContractData) > 0 {
					scInteractionType := getSmartContractInteraction(txData.ToAddress, txData.ContractData, verbose, log)
					if scInteractionType != "Smart Contract" {
						logEvent = logEvent.Str("SCTXType", scInteractionType)
					}
				}
				
				logEvent.
					Str("from", hexToBase58Address(txData.FromAddress)).
					Str("to", hexToBase58Address(txData.ToAddress)).
					Int64("amount", txData.Amount).
					Bool("success", txData.Success).
					Msg("Transaction in block")
			}
		}
	}

	// Store all unique addresses
	for _, addrInfo := range blockEvent.Addresses {
		if err := storeAddressInfo(ctx, addrInfo, blockEvent, blockTimestamp, db, log, verbose); err != nil {
			log.Error().
				Err(err).
				Str("address", addrInfo.Address).
				Msg("Failed to store address info")
			// Continue processing other addresses
		}
	}

	// Store all transactions
	for _, txData := range blockEvent.Transactions {
		if err := storeTransactionData(ctx, txData, blockEvent, db, log, verbose); err != nil {
			log.Error().
				Err(err).
				Str("txHash", txData.TxHash).
				Msg("Failed to store transaction")
			// Continue processing other transactions
		}
	}

	return nil
}

// storeAddressInfo stores or updates address information
func storeAddressInfo(ctx context.Context, addrInfo *monitor.AddressInfo, blockEvent *monitor.BlockEvent, timestamp time.Time, db *storage.Database, log *logger.Logger, verbose bool) error {
	// Check if address exists
	existing, err := db.AddressRepo.FindByAddress(ctx, addrInfo.Address)
	if err != nil {
		return fmt.Errorf("failed to check existing address: %w", err)
	}

	if existing == nil {
		// Create new address
		addr := &models.Address{
			Address:      addrInfo.Address,
			Network:      defaultNetwork,
			Balance:      addrInfo.Balance,
			Type:         addrInfo.Type,
			FirstSeen:    timestamp,
			LastActivity: timestamp,
			TxCount:      int64(addrInfo.TxCount),
			Metadata:     make(map[string]interface{}),
		}

		// Store interaction details in metadata
		addr.Metadata["interactions"] = addrInfo.Interactions
		addr.Metadata["incomingTx"] = addrInfo.IncomingTx
		addr.Metadata["outgoingTx"] = addrInfo.OutgoingTx
		addr.Metadata["contractCalls"] = addrInfo.ContractCalls
		if addrInfo.ContractAddress != "" {
			addr.Metadata["contractAddress"] = addrInfo.ContractAddress
		}

		if err := db.AddressRepo.Create(ctx, addr); err != nil {
			if !isDuplicateKeyError(err) {
				return fmt.Errorf("failed to create address: %w", err)
			}
			log.Debug().Str("address", hexToBase58Address(addrInfo.Address)).Msg("Address already exists")
		} else {
			if verbose {
				log.Info().
					Str("address", hexToBase58Address(addrInfo.Address)).
					Str("addressHex", addrInfo.Address).
					Str("type", addrInfo.Type).
					Int64("balance", addrInfo.Balance).
					Int("txCount", addrInfo.TxCount).
					Int("interactions", len(addrInfo.Interactions)).
					Int("incomingTx", addrInfo.IncomingTx).
					Int("outgoingTx", addrInfo.OutgoingTx).
					Int("contractCalls", addrInfo.ContractCalls).
					Interface("interactionDetails", addrInfo.Interactions).
					Msg("New address stored")
			} else {
				log.Debug().
					Str("address", hexToBase58Address(addrInfo.Address)).
					Str("type", addrInfo.Type).
					Int("txCount", addrInfo.TxCount).
					Msg("New address stored")
			}
		}
	} else {
		// Update existing address
		if err := db.AddressRepo.UpdateActivity(ctx, addrInfo.Address, timestamp); err != nil {
			log.Warn().Err(err).Str("address", addrInfo.Address).Msg("Failed to update address activity")
		}

		// Update balance if changed
		if addrInfo.Balance != existing.Balance {
			if err := db.AddressRepo.UpdateBalance(ctx, addrInfo.Address, addrInfo.Balance); err != nil {
				log.Warn().Err(err).Str("address", addrInfo.Address).Msg("Failed to update balance")
			}
		}
	}

	return nil
}

// storeTransactionData stores comprehensive transaction data
func storeTransactionData(ctx context.Context, txData *monitor.TransactionData, blockEvent *monitor.BlockEvent, db *storage.Database, log *logger.Logger, verbose bool) error {
	// Check if transaction already exists
	existing, err := db.TransactionRepo.FindByHash(ctx, txData.TxHash)
	if err != nil {
		return fmt.Errorf("failed to check existing transaction: %w", err)
	}

	if existing != nil {
		log.Debug().Str("txHash", txData.TxHash).Msg("Transaction already exists")
		return nil
	}

	// Create transaction record
	tx := &models.Transaction{
		TxHash:         txData.TxHash,
		TxID:           txData.TxID,
		Network:        defaultNetwork,
		BlockNumber:    blockEvent.BlockNumber,
		BlockHash:      blockEvent.BlockHash,
		BlockTimestamp: blockEvent.BlockTimestamp,
		FromAddress:    txData.FromAddress,
		ToAddress:      txData.ToAddress,
		Amount:         txData.Amount,
		ContractType:   txData.ContractType,
		Success:        txData.Success,
		EnergyUsage:    txData.EnergyUsage,
		EnergyFee:      txData.EnergyFee,
		NetUsage:       txData.NetUsage,
		NetFee:         txData.NetFee,
		RawData:        make(map[string]interface{}),
	}

	// Store additional data in RawData
	if len(txData.ContractData) > 0 {
		tx.RawData["contractData"] = txData.ContractData
	}
	if len(txData.Logs) > 0 {
		tx.RawData["logs"] = txData.Logs
	}
	if len(txData.InternalTxs) > 0 {
		tx.RawData["internalTransactions"] = txData.InternalTxs
	}

	if err := db.TransactionRepo.Create(ctx, tx); err != nil {
		if !isDuplicateKeyError(err) {
			return fmt.Errorf("failed to create transaction: %w", err)
		}
		return nil
	}

	if verbose {
		tronTxType := getTransactionTypeDisplay(txData.ContractType)
		
		logEvent := log.Info().
			Str("txHash", txData.TxHash).
			Str("TronTXType", tronTxType).
			Str("contractType", txData.ContractType)
		
		// If it's a smart contract interaction, try to decode it and add SCTXType
		if txData.ContractType == "TriggerSmartContract" && len(txData.ContractData) > 0 {
			scInteractionType := getSmartContractInteraction(txData.ToAddress, txData.ContractData, verbose, log)
			if scInteractionType != "Smart Contract" {
				logEvent = logEvent.Str("SCTXType", scInteractionType)
			}
		}
		
		logEvent.
			Str("from", hexToBase58Address(txData.FromAddress)).
			Str("fromHex", txData.FromAddress).
			Str("to", hexToBase58Address(txData.ToAddress)).
			Str("toHex", txData.ToAddress).
			Int64("amount", txData.Amount).
			Bool("success", txData.Success).
			Int64("energyUsage", txData.EnergyUsage).
			Int64("netUsage", txData.NetUsage).
			Int("logsCount", len(txData.Logs)).
			Int("internalTxCount", len(txData.InternalTxs)).
			Interface("logs", txData.Logs).
			Interface("internalTxs", txData.InternalTxs).
			Interface("contractData", txData.ContractData).
			Msg("Transaction stored")
	}

	// Create events from transaction logs
	for i, logData := range txData.Logs {
		evt := &models.Event{
			EventID:        fmt.Sprintf("%s-log-%d", txData.TxHash, i),
			Network:        defaultNetwork,
			Type:           "ContractLog",
			Address:        fmt.Sprintf("%v", logData["address"]),
			TxHash:         txData.TxHash,
			BlockNumber:    blockEvent.BlockNumber,
			BlockTimestamp: blockEvent.BlockTimestamp,
			Data:           logData,
			Processed:      false,
		}

		if err := db.EventRepo.Create(ctx, evt); err != nil {
			if !isDuplicateKeyError(err) {
				log.Debug().
					Err(err).
					Str("eventID", evt.EventID).
					Msg("Failed to create event")
			}
		}
	}

	log.Debug().
		Str("txHash", txData.TxHash).
		Str("type", txData.ContractType).
		Str("from", txData.FromAddress).
		Str("to", txData.ToAddress).
		Bool("success", txData.Success).
		Msg("Transaction stored")

	return nil
}

// isDuplicateKeyError checks if the error is a MongoDB duplicate key error
func isDuplicateKeyError(err error) bool {
	return err != nil && (
		containsString(err.Error(), "duplicate key error") ||
		containsString(err.Error(), "E11000"))
}

// containsString checks if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || 
		len(s) > len(substr) && findSubstring(s, substr))
}

// findSubstring is a simple substring search
func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
