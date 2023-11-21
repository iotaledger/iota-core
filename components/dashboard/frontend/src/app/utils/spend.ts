export function resolveBase58SpendID(base58Spend: string): string {
    switch (base58Spend) {
        case MasterSpendInBase58:
            return "MasterSpendID";
        case UndefinedSpendInBase58:
            return "UndefinedSpendID";
        case LazyBookedSpendsSpendInBase58:
            return "LazyBookedSpendsSpendID";
        case InvalidSpendInBase58:
            return "InvalidSpendID";
        default:
            // otherwise it is a "regular" spendID that doesn't have a distinct name
            return base58Spend
    }
}

// base58 spendIDs that have distinct names
const MasterSpendInBase58 = "4uQeVj5tqViQh7yWWGStvkEG1Zmhx6uasJtWCJziofM"
const UndefinedSpendInBase58 = "11111111111111111111111111111111"
const LazyBookedSpendsSpendInBase58 = "JEKNVnkbo3jma5nREBBJCDoXFVeKkD56V3xKrvRmWxFF"
const InvalidSpendInBase58 = "JEKNVnkbo3jma5nREBBJCDoXFVeKkD56V3xKrvRmWxFG"