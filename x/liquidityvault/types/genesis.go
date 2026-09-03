package types

import (
	"fmt"

	"cosmossdk.io/math"
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
	for _, vault := range gs.Vaults {
		if err := vault.Validate(); err != nil {
			return err
		}
		if seen[vault.ValidatorAddress] {
			return fmt.Errorf("duplicate vault for validator %s", vault.ValidatorAddress)
		}
		seen[vault.ValidatorAddress] = true
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
		key := positionKey{position.ValidatorAddress, position.PoolId}
		if _, exists := positionShares[key]; exists {
			return fmt.Errorf("duplicate position for validator %s in pool %d", position.ValidatorAddress, position.PoolId)
		}
		positionShares[key] = position.Shares
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
