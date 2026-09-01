package types

// liquidityvault module event types and attribute keys
const (
	EventTypeDeposit             = "vault_deposit"
	EventTypeWithdrawalInitiated = "vault_withdrawal_initiated"
	EventTypeWithdrawalCompleted = "vault_withdrawal_completed"
	EventTypeSetRewardShare      = "vault_set_reward_share"

	AttributeKeyValidator            = "validator"
	AttributeKeyAmount               = "amount"
	AttributeKeyCompleteTime         = "complete_time"
	AttributeKeyDelegatorRewardShare = "delegator_reward_share"
)
