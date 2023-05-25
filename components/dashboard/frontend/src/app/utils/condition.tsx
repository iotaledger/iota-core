import { UnlockConditionAddress, UnlockConditionExpiration, UnlockConditionStorageDepositReturn, UnlockConditionTimelock } from "app/components/Condition";
import { AddressUnlockCondition, Condition, ExpirationUnlockCondition, StorageDepositReturnUnlockCondition, TimelockUnlockCondition } from "app/misc/Payload";
import * as React from 'react';

export enum ConditionType {
    Address = 0,
    StorageDepositReturn,
    Timelock,
    Expiration,
    StateControllerAddress,
    GovernorAddress,
    ImmutableAlias,
}

export function resolveConditionType(condType: number) {
    switch (condType) {
        case ConditionType.Address:
            return "Address Condition";
        case ConditionType.StorageDepositReturn:
            return "StorageDepositReturn Condition";
        case ConditionType.Timelock:
            return "Timelock Condition";
        case ConditionType.Expiration:
            return "Expiration Condition";
        case ConditionType.StateControllerAddress:
            return "StateControllerAddress Condition";
        case ConditionType.GovernorAddress:
            return "GovernorAddress Condition";
        case ConditionType.ImmutableAlias:
            return "ImmutableAlias Condition";
        default:
            return "Unknown Condition Type";
    }
}

export function conditionToComponent(cond: Condition) {
    switch (cond.type) {
        case ConditionType.Address:
        case ConditionType.StateControllerAddress:
        case ConditionType.GovernorAddress:
            return <UnlockConditionAddress unlock={cond.condition as AddressUnlockCondition} />;
        case ConditionType.StorageDepositReturn:
            return <UnlockConditionStorageDepositReturn unlock={cond.condition as StorageDepositReturnUnlockCondition} />;
        case ConditionType.Expiration:
            return <UnlockConditionExpiration unlock={cond.condition as ExpirationUnlockCondition} />;
        case ConditionType.Timelock:
            return <UnlockConditionTimelock unlock={cond.condition as TimelockUnlockCondition} />;
    }
}