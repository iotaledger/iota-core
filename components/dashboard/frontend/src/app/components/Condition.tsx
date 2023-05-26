import * as React from 'react';
import { 
    TimelockUnlockCondition, 
    AddressUnlockCondition,
    StorageDepositReturnUnlockCondition,
ExpirationUnlockCondition} from "../misc/Payload";
import ListGroup from "react-bootstrap/ListGroup";

interface AddressProps { 
    unlock: AddressUnlockCondition;
}

export class UnlockConditionAddress extends React.Component<AddressProps, any> {
    render() {
        return (
            <div className={"mb-2"} key={"unlockCondition"+this.props.unlock.address}>
                <ListGroup>
                    <ListGroup.Item>Address: {this.props.unlock.address}</ListGroup.Item>
                </ListGroup>
            </div>
        );
    }
}

interface TimelockProps { 
    unlock: TimelockUnlockCondition;
}

export class UnlockConditionTimelock extends React.Component<TimelockProps, any> {
    render() {

        return (
            this.props.unlock.unixTime &&
            <div className={"mb-2"} key={"unlockCondition"+this.props.unlock.unixTime}>
                <ListGroup>
                    <ListGroup.Item>Unix Time: {this.props.unlock.unixTime}</ListGroup.Item>
                </ListGroup>
            </div>
        );
    }
}

interface StorageDepositReturnProps { 
    unlock: StorageDepositReturnUnlockCondition;
}

export class UnlockConditionStorageDepositReturn extends React.Component<StorageDepositReturnProps, any> {
    render() {

        return (
            <div className={"mb-2"} key={"unlockCondition"+this.props.unlock.returnAddress}>
                <ListGroup>
                    <ListGroup.Item>Return Address: {this.props.unlock.returnAddress}</ListGroup.Item>
                    <ListGroup.Item>Amount: {this.props.unlock.amount}</ListGroup.Item>
                </ListGroup>
            </div>
        );
    }
}

interface ExpirationProps { 
    unlock: ExpirationUnlockCondition;
}

export class UnlockConditionExpiration extends React.Component<ExpirationProps, any> {
    render() {

        return (
            <div className={"mb-2"} key={"unlockCondition"+this.props.unlock.returnAddress}>
                <ListGroup>
                    <ListGroup.Item>Return Address: {this.props.unlock.returnAddress}</ListGroup.Item>
                    <ListGroup.Item>Unix Time: {this.props.unlock.unixTime}</ListGroup.Item>
                </ListGroup>
            </div>
        );
    }
}