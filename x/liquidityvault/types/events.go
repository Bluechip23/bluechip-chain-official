package types

// liquidityvault module event types and attribute keys
const (
	EventTypeDeposit               = "vault_deposit"
	EventTypeWithdrawalInitiated   = "vault_withdrawal_initiated"
	EventTypeWithdrawalCompleted   = "vault_withdrawal_completed"
	EventTypeSetRewardShare        = "vault_set_reward_share"
	EventTypePoolRegistered        = "vault_pool_registered"
	EventTypePoolEnabledSet        = "vault_pool_enabled_set"
	EventTypeAllocation            = "vault_allocation"
	EventTypeDeallocationInitiated = "vault_deallocation_initiated"
	EventTypeDeallocationCompleted = "vault_deallocation_completed"
	EventTypeDeallocationRequeued  = "vault_deallocation_requeued"
	EventTypeDeallocationAbandoned = "vault_deallocation_abandoned"

	AttributeKeyValidator            = "validator"
	AttributeKeyAmount               = "amount"
	AttributeKeyCompleteTime         = "complete_time"
	AttributeKeyDelegatorRewardShare = "delegator_reward_share"
	AttributeKeyPoolID               = "pool_id"
	AttributeKeyContract             = "contract"
	AttributeKeyEnabled              = "enabled"
	AttributeKeyShares               = "shares"
	AttributeKeyError                = "error"
)
