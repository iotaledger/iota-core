export enum SignatureType {
    Ed25519 = 0,
    BLS = 1,
}

export function resolveSignatureType(sigType: number) {
    switch (sigType) {
        case SignatureType.Ed25519:
            return "Ed25519 Signature";
        case SignatureType.BLS:
            return "BLS Signature";
        default:
            return "Unknown Signature Type";
    }
}