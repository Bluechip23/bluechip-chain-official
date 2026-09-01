package keeper

import (
	"context"

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

	staked, vaultBalance, score, err := k.GetCompositeScore(goCtx, valAddr)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	return &types.QueryCompositeScoreResponse{
		StakedTokens:   staked,
		VaultBalance:   vaultBalance,
		CompositeScore: score,
	}, nil
}
