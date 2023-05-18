import * as React from 'react';
import {OutputID, SigLockedColoredOutput} from "../misc/Payload";
import Badge from "react-bootstrap/Badge";
import ListGroup from "react-bootstrap/ListGroup";
import {resolveColor} from "../utils/color";

interface Props {
    output: SigLockedColoredOutput
    id: OutputID;
}

export class SigLockedColoredOutputComponent extends React.Component<Props, any> {
    render() {
        let balances = Object.keys(this.props.output.balances).map((key) => {return {color:key, value:this.props.output.balances[key]}})
        return (
            <div className={"mb-2"} key={this.props.id.hex}>
                <ListGroup>
                    <ListGroup.Item>Type: SigLockerColoredOutput</ListGroup.Item>
                    <ListGroup.Item>
                        Balances:
                        <div>
                            {balances.map((entry, i) => (<div key={i}><Badge variant="success">{new Intl.NumberFormat().format(entry.value)} {resolveColor(entry.color)}</Badge></div>))}
                        </div>
                    </ListGroup.Item>
                    <ListGroup.Item>OutputID: <a href={`/explorer/output/${this.props.id.hex}`}>{this.props.id.hex}</a></ListGroup.Item>
                    <ListGroup.Item>Address: <a href={`/explorer/address/${this.props.output.address}`}> {this.props.output.address}</a></ListGroup.Item>
                <ListGroup.Item>Transaction: <a href={`/explorer/transaction/${this.props.id.transactionID}`}> {this.props.id.transactionID}</a></ListGroup.Item>
                <ListGroup.Item>Output Index: {this.props.id.outputIndex}</ListGroup.Item>
                </ListGroup>
            </div>
        );
    }
}