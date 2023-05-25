import * as React from 'react';
import { 
IssuerFeature, MetadataFeature, TagFeature} from "../misc/Payload";
import ListGroup from "react-bootstrap/ListGroup";

interface IssuerProps { 
    feature: IssuerFeature;
}

export class FeatureIssuer extends React.Component<IssuerProps, any> {
    render() {
        return (
            <div className={"mb-2"} key={"feature"+this.props.feature.address}>
                <ListGroup>
                    <ListGroup.Item>Address: {this.props.feature.address}</ListGroup.Item>
                </ListGroup>
            </div>
        );
    }
}


interface MetadataProps { 
    feature: MetadataFeature;
}

export class FeatureMetadata extends React.Component<MetadataProps, any> {
    render() {
        return (
            <div className={"mb-2"} key={"feature"+this.props.feature.data}>
                <ListGroup>
                    <ListGroup.Item>Data: {this.props.feature.data}</ListGroup.Item>
                </ListGroup>
            </div>
        );
    }
}

interface TagProps { 
    feature: TagFeature;
}

export class FeatureTag extends React.Component<TagProps, any> {
    render() {
        return (
            <div className={"mb-2"} key={"feature"+this.props.feature.tag}>
                <ListGroup>
                    <ListGroup.Item>Data: {this.props.feature.tag}</ListGroup.Item>
                </ListGroup>
            </div>
        );
    }
}