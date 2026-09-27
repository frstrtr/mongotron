package subscription

import "github.com/frstrtr/mongotron/internal/storage/models"

// ContractTypesForAssets returns the contract types to monitor for the given asset
// types. An empty list means all transfer types (TRX, TRC10, TRC20).
func ContractTypesForAssets(assetTypes []string) []string {
	if len(assetTypes) == 0 {
		// Default: monitor all transfer types
		return []string{"TransferContract", "TransferAssetContract", "TriggerSmartContract"}
	}

	contractTypes := make([]string, 0, 10)
	for _, asset := range assetTypes {
		switch asset {
		case "TRX":
			if !containsString(contractTypes, "TransferContract") {
				contractTypes = append(contractTypes, "TransferContract")
			}
		case "TRC10":
			if !containsString(contractTypes, "TransferAssetContract") {
				contractTypes = append(contractTypes, "TransferAssetContract")
			}
		case "TRC20":
			if !containsString(contractTypes, "TriggerSmartContract") {
				contractTypes = append(contractTypes, "TriggerSmartContract")
			}
		case "*":
			// All transfer types (legacy)
			return []string{"TransferContract", "TransferAssetContract", "TriggerSmartContract"}

		// Staking operations
		case "STAKE", "FREEZE":
			if !containsString(contractTypes, "FreezeBalanceV2Contract") {
				contractTypes = append(contractTypes, "FreezeBalanceV2Contract")
			}
		case "UNSTAKE", "UNFREEZE":
			if !containsString(contractTypes, "UnfreezeBalanceV2Contract") {
				contractTypes = append(contractTypes, "UnfreezeBalanceV2Contract")
			}
		case "WITHDRAW_UNSTAKE":
			if !containsString(contractTypes, "WithdrawExpireUnfreezeContract") {
				contractTypes = append(contractTypes, "WithdrawExpireUnfreezeContract")
			}

		// Delegation operations
		case "DELEGATE":
			if !containsString(contractTypes, "DelegateResourceContract") {
				contractTypes = append(contractTypes, "DelegateResourceContract")
			}
		case "UNDELEGATE":
			if !containsString(contractTypes, "UnDelegateResourceContract") {
				contractTypes = append(contractTypes, "UnDelegateResourceContract")
			}

		// Voting operations
		case "VOTE":
			if !containsString(contractTypes, "VoteWitnessContract") {
				contractTypes = append(contractTypes, "VoteWitnessContract")
			}

		// Permission operations (CRITICAL for security)
		case "PERMISSION":
			if !containsString(contractTypes, "AccountPermissionUpdateContract") {
				contractTypes = append(contractTypes, "AccountPermissionUpdateContract")
			}

		// Claim voting rewards
		case "CLAIM":
			if !containsString(contractTypes, "WithdrawBalanceContract") {
				contractTypes = append(contractTypes, "WithdrawBalanceContract")
			}

		// All operations for full gas station monitoring
		case "ALL_OPERATIONS", "FULL":
			return []string{
				"TransferContract",
				"TransferAssetContract",
				"TriggerSmartContract",
				"FreezeBalanceV2Contract",
				"UnfreezeBalanceV2Contract",
				"WithdrawExpireUnfreezeContract",
				"DelegateResourceContract",
				"UnDelegateResourceContract",
				"VoteWitnessContract",
				"AccountPermissionUpdateContract",
				"WithdrawBalanceContract",
			}
		}
	}

	if len(contractTypes) == 0 {
		// Fallback to all transfer types
		return []string{"TransferContract", "TransferAssetContract", "TriggerSmartContract"}
	}

	return contractTypes
}

// FiltersForAssets builds the subscription filters a watchlist entry uses for the
// given asset types and token filter: only successful transactions of the matching
// contract types. It is the single source of truth for create and update.
func FiltersForAssets(assetTypes, tokenFilter []string) models.SubscriptionFilters {
	return models.SubscriptionFilters{
		ContractTypes: ContractTypesForAssets(assetTypes),
		AssetTypes:    assetTypes,
		TokenFilter:   tokenFilter,
		OnlySuccess:   true,
	}
}

func containsString(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
