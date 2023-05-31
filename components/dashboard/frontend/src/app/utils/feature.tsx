import { FeatureIssuer, FeatureMetadata, FeatureTag } from "app/components/Feature";
import { Feature, IssuerFeature, MetadataFeature, TagFeature } from "app/misc/Payload";
import * as React from 'react';

export enum FeatureType {
    Sender = 0,
    Issuer,
    Metadata,
    Tag,
}

export function resolveSignatureType(sigType: number) {
    switch (sigType) {
        case FeatureType.Sender:
            return "Sender Feature";
        case FeatureType.Issuer:
            return "Issuer Feature";
        case FeatureType.Metadata:
            return "Metadata Feature";
        case FeatureType.Tag:
            return "Tag Feature";
        default:
            return "Unknown Feature Type";
    }
}


export function featureToComponent(feat: Feature) {
    switch (feat.type) {
        case FeatureType.Sender:
        case FeatureType.Issuer:
            return <FeatureIssuer feature={feat.feature as IssuerFeature} />;
        case FeatureType.Metadata:
            return <FeatureMetadata feature={feat.feature as MetadataFeature} />;
        case FeatureType.Tag:
            return <FeatureTag feature={feat.feature as TagFeature} />;
    }
}