package keeper

import (
	"context"
	"sort"

	"cosmossdk.io/math"
	"cosmossdk.io/store/prefix"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"bluechipChain/x/liquidityvault/types"
)

var _ types.QueryServer = Keeper{}

// Params queries the parameters of the module.
func (k Keeper) Params(goCtx context.Context, req *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	return &types.QueryParamsResponse{Params: k.GetParams(goCtx)}, nil
}

// Vault queries a validator's Liquidity Vault and its pending withdrawals.
func (k Keeper) Vault(goCtx context.Context, req *types.QueryVaultRequest) (*types.QueryVaultResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}
	valAddr, err := sdk.ValAddressFromBech32(req.ValidatorAddress)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid validator address")
	}

	vault, found := k.GetVault(goCtx, valAddr)
	if !found {
		return nil, status.Error(codes.NotFound, "validator has no liquidity vault")
	}

	return &types.QueryVaultResponse{
		Vault:              vault,
		PendingWithdrawals: k.GetPendingWithdrawals(goCtx, valAddr),
	}, nil
}

// Vaults queries all Liquidity Vaults.
func (k Keeper) Vaults(goCtx context.Context, req *types.QueryVaultsRequest) (*types.QueryVaultsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(goCtx))
	vaultStore := prefix.NewStore(store, types.VaultKeyPrefix)

	var vaults []types.Vault
	pageRes, err := query.Paginate(vaultStore, req.Pagination, func(key []byte, value []byte) error {
		var vault types.Vault
		if err := k.cdc.Unmarshal(value, &vault); err != nil {
			return err
		}
		vaults = append(vaults, vault)
		return nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryVaultsResponse{Vaults: vaults, Pagination: pageRes}, nil
}

// CompositeScore queries a validator's composite score.
func (k Keeper) CompositeScore(goCtx context.Context, req *types.QueryCompositeScoreRequest) (*types.QueryCompositeScoreResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}
	valAddr, err := sdk.ValAddressFromBech32(req.ValidatorAddress)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid validator address")
	}

	score, err := k.GetCompositeScore(goCtx, valAddr)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	return &types.QueryCompositeScoreResponse{
		StakedTokens:     score.StakedTokens,
		VaultBalance:     score.VaultBalance,
		PositionValue:    score.PositionValue,
		MedianVaultValue: score.MedianVaultValue,
		CompositeScore:   score.Score,
	}, nil
}

// ValuePosts queries a validator's value-post window and its median.
func (k Keeper) ValuePosts(goCtx context.Context, req *types.QueryValuePostsRequest) (*types.QueryValuePostsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}
	valAddr, err := sdk.ValAddressFromBech32(req.ValidatorAddress)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid validator address")
	}

	history := k.GetValuePostHistory(goCtx, valAddr)
	resp := &types.QueryValuePostsResponse{
		Posts:  history.Posts,
		Median: MedianOfPosts(history.Posts),
	}
	if next, found := k.NextScheduledValuePost(goCtx, valAddr); found {
		postTime := next.PostTime
		resp.NextPostTime = &postTime
	}
	return resp, nil
}

// SetRanking queries the shadow complex-check ranking: validators ordered
// by staked tokens with the composite score as tiebreaker. Observability
// only; it has no effect on the actual validator set.
func (k Keeper) SetRanking(goCtx context.Context, req *types.QuerySetRankingRequest) (*types.QuerySetRankingResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	validators, err := k.stakingKeeper.GetAllValidators(goCtx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	sdkCtx := sdk.UnwrapSDKContext(goCtx)
	ranked := make([]types.RankedValidator, 0, len(validators))
	for _, validator := range validators {
		valAddr, err := sdk.ValAddressFromBech32(validator.GetOperator())
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		// Same definition as GetCompositeScore's vault component; dark
		// pools degrade to their cached values so one broken contract
		// cannot take down the whole ranking.
		vaultBalance := math.ZeroInt()
		positionValue := math.ZeroInt()
		if vault, found := k.GetVault(goCtx, valAddr); found {
			vaultBalance = vault.Balance
			positionValue, err = k.positionsValueWithFallback(sdkCtx, valAddr, false)
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
		}
		median := k.medianVaultValueOrLive(sdkCtx, valAddr, vaultBalance, positionValue)

		staked := validator.GetTokens()
		ranked = append(ranked, types.RankedValidator{
			ValidatorAddress: validator.GetOperator(),
			StakedTokens:     staked,
			MedianVaultValue: median,
			CompositeScore:   staked.Add(median),
		})
	}

	// The design document's complex check: staked tokens first, composite
	// score as the tiebreaker; address as a final deterministic tiebreak.
	sort.SliceStable(ranked, func(i, j int) bool {
		if !ranked[i].StakedTokens.Equal(ranked[j].StakedTokens) {
			return ranked[i].StakedTokens.GT(ranked[j].StakedTokens)
		}
		if !ranked[i].CompositeScore.Equal(ranked[j].CompositeScore) {
			return ranked[i].CompositeScore.GT(ranked[j].CompositeScore)
		}
		return ranked[i].ValidatorAddress < ranked[j].ValidatorAddress
	})

	return &types.QuerySetRankingResponse{Validators: ranked}, nil
}

// DelegatorReward queries a delegator's claimable vault rewards from one
// validator.
func (k Keeper) DelegatorReward(goCtx context.Context, req *types.QueryDelegatorRewardRequest) (*types.QueryDelegatorRewardResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}
	delAddr, err := sdk.AccAddressFromBech32(req.DelegatorAddress)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid delegator address")
	}
	valAddr, err := sdk.ValAddressFromBech32(req.ValidatorAddress)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid validator address")
	}

	return &types.QueryDelegatorRewardResponse{
		Claimable: k.ClaimableReward(goCtx, delAddr, valAddr),
	}, nil
}

// Pools queries all registered liquidity pools.
func (k Keeper) Pools(goCtx context.Context, req *types.QueryPoolsRequest) (*types.QueryPoolsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(goCtx))
	poolStore := prefix.NewStore(store, types.PoolKeyPrefix)

	var pools []types.RegisteredPool
	pageRes, err := query.Paginate(poolStore, req.Pagination, func(key []byte, value []byte) error {
		var pool types.RegisteredPool
		if err := k.cdc.Unmarshal(value, &pool); err != nil {
			return err
		}
		pools = append(pools, pool)
		return nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryPoolsResponse{Pools: pools, Pagination: pageRes}, nil
}

// Positions queries a validator's pool positions with current values.
func (k Keeper) Positions(goCtx context.Context, req *types.QueryPositionsRequest) (*types.QueryPositionsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}
	valAddr, err := sdk.ValAddressFromBech32(req.ValidatorAddress)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid validator address")
	}

	var views []types.PositionView
	for _, position := range k.GetValidatorPositions(goCtx, valAddr) {
		pool, found := k.GetPool(goCtx, position.PoolId)
		if !found {
			return nil, status.Errorf(codes.Internal, "position references unregistered pool %d", position.PoolId)
		}
		poolValue, err := k.PoolPositionValue(goCtx, pool)
		if err != nil {
			return nil, status.Error(codes.Unavailable, err.Error())
		}
		views = append(views, types.PositionView{
			Position: position,
			Value:    positionValueFromShares(position.Shares, k.GetPoolTotalShares(goCtx, position.PoolId), poolValue),
		})
	}

	return &types.QueryPositionsResponse{
		Positions:            views,
		PendingDeallocations: k.GetPendingDeallocations(goCtx, valAddr),
	}, nil
}
