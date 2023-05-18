import * as React from 'react';
import { NativeToken as NativeTokenJSON} from "../misc/Payload";
import ListGroup from "react-bootstrap/ListGroup";


interface Props {
    token: NativeTokenJSON;
}

export class NativeToken extends React.Component<Props, any> {
    render() {
        return (
            <div className={"mb-2"} key={this.props.token.id}>
                <ListGroup>
                    <ListGroup.Item>ID: {this.props.token.id}</ListGroup.Item>
                    <ListGroup.Item>Amount: {this.props.token.amount}</ListGroup.Item>
                </ListGroup>
            </div>
        );
    }
}



