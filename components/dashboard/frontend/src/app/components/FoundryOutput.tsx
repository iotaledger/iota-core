import * as React from 'react';
import {OutputID, FoundryOutput as FoundryOutputJSON} from "../misc/Payload";
import ListGroup from "react-bootstrap/ListGroup";
import { NativeToken } from './NativeToken';
import { conditionToComponent } from 'app/utils/condition';
import { featureToComponent } from 'app/utils/feature';

interface Props {
    output: FoundryOutputJSON
    id: OutputID;
}

export class FoundryOutput extends React.Component<Props, any> {
    render() {
        return (
            <div className={"mb-2"} key={this.props.id.hex}>
                <ListGroup>
                    <ListGroup.Item>Type: FoundryOutput</ListGroup.Item>
                    <ListGroup.Item>Amount: {this.props.output.amount}</ListGroup.Item>
                    <ListGroup.Item>OutputID: <a href={`/explorer/output/${this.props.id.hex}`}>{this.props.id.hex}</a></ListGroup.Item>
                    <ListGroup.Item>Serial Number: {this.props.output.serialNumber}</ListGroup.Item>
                    <ListGroup.Item>Token Scheme: {this.props.output.tokenScheme}</ListGroup.Item>   
                    {
                        this.props.output.nativeTokens && this.props.output.nativeTokens.map((n, _) => <NativeToken token={n} />)
                    }
                    {
                        this.props.output.unlockConditions && this.props.output.unlockConditions.map((c, _) => conditionToComponent(c))
                    }
                    {
                        this.props.output.features && this.props.output.features.map((f, _) => featureToComponent(f))
                    }
                    <ListGroup.Item>Transaction: <a href={`/explorer/transaction/${this.props.id.transactionID}`}> {this.props.id.transactionID}</a></ListGroup.Item>
                    <ListGroup.Item>Output Index: {this.props.id.outputIndex}</ListGroup.Item>
                </ListGroup>
            </div>
        );
    }
}



