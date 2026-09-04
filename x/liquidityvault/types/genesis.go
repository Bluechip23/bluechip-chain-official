package types

import (
	"fmt"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// DefaultGenesis returns the default genesis state
func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params: DefaultParams(),
	}
}

// Validate performs basic genesis state validation returning an error upon any
// failure.
func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}

	seen := make(map[string]bool, len(gs.Vaults))
	vaultIndexes := make(map[string]math.LegacyDec, len(gs.Vaults))
	for _, vault := range gs.Vaults {
		if err := vault.Validate(); err != nil {
			return err
		}
		if seen[vault.ValidatorAddress] {
			return fmt.Errorf("duplicate vault for validator %s", vault.ValidatorAddress)
		}
		seen[vault.ValidatorAddress] = true
		vaultIndexes[vault.ValidatorAddress] = vault.RewardIndex
	}

	for _, withdrawal := range gs.PendingWithdrawals {
		if err := withdrawal.Validate(); err != nil {
			return err
		}
	}

	poolIDs := make(map[uint64]bool, len(gs.Pools))
	poolContracts := make(map[string]bool, len(gs.Pools))
	maxPoolID := uint64(0)
	for _, pool := range gs.Pools {
		if err := pool.Validate(); err != nil {
			return err
		}
		if poolIDs[pool.PoolId] {
			return fmt.Errorf("duplicate pool id %d", pool.PoolId)
		}
		if poolContracts[pool.ContractAddress] {
			return fmt.Errorf("duplicate pool contract %s", pool.ContractAddress)
		}
		poolIDs[pool.PoolId] = true
		poolContracts[pool.ContractAddress] = true
		if pool.PoolId > maxPoolID {
			maxPoolID = pool.PoolId
		}
	}
	if len(gs.Pools) > 0 && gs.NextPoolId <= maxPoolID {
		return fmt.Errorf("next pool id %d must exceed the highest registered pool id %d", gs.NextPoolId, maxPoolID)
	}

	type positionKey struct {
		validator string
		poolID    uint64
	}
	positionShares := make(map[positionKey]math.Int, len(gs.Positions))
	for _, position := range gs.Positions {
		if err := position.Validate(); err != nil {
			return err
		}
		if !poolIDs[position.PoolId] {
			return fmt.Errorf("position references unregistered pool %d", position.PoolId)
		}
		if !seen[position.ValidatorAddress] {
			return fmt.Errorf("position for validator %s has no vault", position.ValidatorAddress)
		}
		key := positionKey{position.ValidatorAddress, position.PoolId}
		if _, exists := positionShares[key]; exists {
			return fmt.Errorf("duplicate position for validator %s in pool %d", position.ValidatorAddress, position.PoolId)
		}
		positionShares[key] = position.Shares
	}

	historySeen := make(map[string]bool, len(gs.ValuePostHistories))
	for _, history := range gs.ValuePostHistories {
		if _, err := sdk.ValAddressFromBech32(history.ValidatorAddress); err != nil {
			return fmt.Errorf("invalid value post history validator: %w", err)
		}
		if !seen[history.ValidatorAddress] {
			return fmt.Errorf("value post history for %s has no vault", history.ValidatorAddress)
		}
		if historySeen[history.ValidatorAddress] {
			return fmt.Errorf("duplicate value post history for validator %s", history.ValidatorAddress)
		}
		historySeen[history.ValidatorAddress] = true
		if len(history.Posts) > ValuePostWindow {
			return fmt.Errorf("value post history for %s exceeds the window of %d", history.ValidatorAddress, ValuePostWindow)
		}
		for _, post := range history.Posts {
			if post.Value.IsNil() || post.Value.IsNegative() {
				return fmt.Errorf("value post for %s must be non-negative", history.ValidatorAddress)
			}
			if post.PostTime.IsZero() {
				return fmt.Errorf("value post for %s must have a post time", history.ValidatorAddress)
			}
		}
	}

	scheduleSeen := make(map[string]bool, len(gs.ScheduledValuePosts))
	for _, entry := range gs.ScheduledValuePosts {
		if _, err := sdk.ValAddressFromBech32(entry.ValidatorAddress); err != nil {
			return fmt.Errorf("invalid scheduled value post validator: %w", err)
		}
		if !seen[entry.ValidatorAddress] {
			return fmt.Errorf("scheduled value post for %s has no vault", entry.ValidatorAddress)
		}
		if scheduleSeen[entry.ValidatorAddress] {
			return fmt.Errorf("duplicate scheduled value post for validator %s", entry.ValidatorAddress)
		}
		scheduleSeen[entry.ValidatorAddress] = true
		if entry.PostTime.IsZero() {
			return fmt.Errorf("scheduled value post for %s must have a post time", entry.ValidatorAddress)
		}
	}

	cachedSeen := make(map[uint64]bool, len(gs.CachedPoolValues))
	for _, cached := range gs.CachedPoolValues {
		if !poolIDs[cached.PoolId] {
			return fmt.Errorf("cached value references unregistered pool %d", cached.PoolId)
		}
		if cachedSeen[cached.PoolId] {
			return fmt.Errorf("duplicate cached value for pool %d", cached.PoolId)
		}
		cachedSeen[cached.PoolId] = true
		if cached.Value.IsNil() || cached.Value.IsNegative() {
			return fmt.Errorf("cached value for pool %d must be non-negative", cached.PoolId)
		}
	}

	rewardSeen := make(map[string]bool, len(gs.DelegatorRewards))
	accruedPerVault := make(map[string]math.LegacyDec)
	for _, reward := range gs.DelegatorRewards {
		if err := reward.Validate(); err != nil {
			return err
		}
		if !seen[reward.ValidatorAddress] {
			return fmt.Errorf("delegator reward for validator %s has no vault", reward.ValidatorAddress)
		}
		if reward.Index.GT(vaultIndexes[reward.ValidatorAddress]) {
			return fmt.Errorf("delegator reward index for %s exceeds the vault's reward index", reward.DelegatorAddress)
		}
		key := reward.DelegatorAddress + "/" + reward.ValidatorAddress
		if rewardSeen[key] {
			return fmt.Errorf("duplicate delegator reward for %s against %s", reward.DelegatorAddress, reward.ValidatorAddress)
		}
		rewardSeen[key] = true

		total, ok := accruedPerVault[reward.ValidatorAddress]
		if !ok {
			total = math.LegacyZeroDec()
		}
		accruedPerVault[reward.ValidatorAddress] = total.Add(reward.Accrued)
	}
	// Settled accruals must be backed by the vault's outstanding rewards, or
	// claims would draw funds the module-account invariant attributes to
	// vault balances and pending withdrawals.
	for _, vault := range gs.Vaults {
		if total, ok := accruedPerVault[vault.ValidatorAddress]; ok && total.GT(vault.OutstandingRewards) {
			return fmt.Errorf("delegator rewards accrued against %s exceed the vault's outstanding rewards", vault.ValidatorAddress)
		}
	}

	pendingShares := make(map[positionKey]math.Int)
	for _, deallocation := range gs.PendingDeallocations {
		if err := deallocation.Validate(); err != nil {
			return err
		}
		if !poolIDs[deallocation.PoolId] {
			return fmt.Errorf("pending deallocation references unregistered pool %d", deallocation.PoolId)
		}
		key := positionKey{deallocation.ValidatorAddress, deallocation.PoolId}
		owned, hasPosition := positionShares[key]
		if !hasPosition {
			return fmt.Errorf("pending deallocation for validator %s has no position in pool %d", deallocation.ValidatorAddress, deallocation.PoolId)
		}
		pending, ok := pendingShares[key]
		if !ok {
			pending = math.ZeroInt()
		}
		pending = pending.Add(deallocation.Shares)
		if pending.GT(owned) {
			return fmt.Errorf("pending deallocations for validator %s in pool %d exceed the position's shares", deallocation.ValidatorAddress, deallocation.PoolId)
		}
		pendingShares[key] = pending
	}

	return nil
}
