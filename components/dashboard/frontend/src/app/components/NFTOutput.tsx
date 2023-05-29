import * as React from 'react';
import {OutputID, NFTOutput as NFTOutputJSON} from "../misc/Payload";
import {conditionToComponent} from "../utils/condition";
import ListGroup from "react-bootstrap/ListGroup";
import { featureToComponent } from 'app/utils/feature';
import { NativeToken } from './NativeToken';

interface Props {
    output: NFTOutputJSON
    id: OutputID;
}

export class NFTOutput extends React.Component<Props, any> {
    render() {
        return (
            <div className={"mb-2"} key={this.props.id.hex}>
                <ListGroup>
                    <ListGroup.Item>Type: FoundryOutput</ListGroup.Item>
                    <ListGroup.Item>Amount: {this.props.output.amount}</ListGroup.Item>
                    <ListGroup.Item>OutputID: <a href={`/explorer/output/${this.props.id.hex}`}>{this.props.id.hex}</a></ListGroup.Item>
                    <ListGroup.Item>NFT ID: {this.props.output.nftId}</ListGroup.Item>
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



