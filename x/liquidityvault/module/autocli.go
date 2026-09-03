package liquidityvault

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

	modulev1 "bluechipChain/api/bluechipchain/liquidityvault"
)

// AutoCLIOptions implements the autocli.HasAutoCLIConfig interface.
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: modulev1.Query_ServiceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "Params",
					Use:       "params",
					Short:     "Shows the parameters of the module",
				},
				{
					RpcMethod:      "Vault",
					Use:            "vault [validator-address]",
					Short:          "Shows a validator's liquidity vault and pending withdrawals",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "validator_address"}},
				},
				{
					RpcMethod: "Vaults",
					Use:       "vaults",
					Short:     "Lists all liquidity vaults",
				},
				{
					RpcMethod:      "CompositeScore",
					Use:            "composite-score [validator-address]",
					Short:          "Shows a validator's composite score (staked tokens + vault balance + position value)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "validator_address"}},
				},
				{
					RpcMethod: "Pools",
					Use:       "pools",
					Short:     "Lists all registered liquidity pools",
				},
				{
					RpcMethod:      "Positions",
					Use:            "positions [validator-address]",
					Short:          "Shows a validator's pool positions and pending deallocations",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "validator_address"}},
				},
				{
					RpcMethod:      "ValuePosts",
					Use:            "value-posts [validator-address]",
					Short:          "Shows a validator's vault value posts, their median, and the next post time",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "validator_address"}},
				},
				{
					RpcMethod: "SetRanking",
					Use:       "set-ranking",
					Short:     "Shows the shadow complex-check ranking (staked tokens, composite score tiebreaker)",
				},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              modulev1.Msg_ServiceDesc.ServiceName,
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod:      "Deposit",
					Use:            "deposit [amount]",
					Short:          "Deposit bond-denom tokens from the validator account into its liquidity vault",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "amount"}},
				},
				{
					RpcMethod:      "InitiateWithdrawal",
					Use:            "initiate-withdrawal [amount]",
					Short:          "Start a grace-period withdrawal from the validator's liquidity vault",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "amount"}},
				},
				{
					RpcMethod:      "SetRewardShare",
					Use:            "set-reward-share [delegator-reward-share]",
					Short:          "Set the fraction of vault rewards passed through to delegators",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "delegator_reward_share"}},
				},
				{
					RpcMethod: "AllocateToPool",
					Use:       "allocate-to-pool [pool-id] [amount]",
					Short:     "Move active vault balance into a registered liquidity pool",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "pool_id"},
						{ProtoField: "amount"},
					},
				},
				{
					RpcMethod: "DeallocateFromPool",
					Use:       "deallocate-from-pool [pool-id] [shares]",
					Short:     "Request removal of pool liquidity after the universal grace period",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "pool_id"},
						{ProtoField: "shares"},
					},
				},
				{
					RpcMethod: "RegisterPool",
					Skip:      true, // skipped because authority gated
				},
				{
					RpcMethod: "SetPoolEnabled",
					Skip:      true, // skipped because authority gated
				},
				{
					RpcMethod: "UpdateParams",
					Skip:      true, // skipped because authority gated
				},
			},
		},
	}
}
