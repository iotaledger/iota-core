# Core Models

This document defines core models for IOTA V3 protocol.

* [Block](#block)
  * [Block Signature](#block-signature)
  * [Block ID](#block-id)
  * [Block Syntactic Validation Rules](#block-syntactic-validation-rules)
* [Slot Index](#slot-index)
* [Slot Commitment](#slot-commitment)
  * [Slot Commitment ID](#slot-commitment-id)
* [Payloads](#payloads)
  * [Tagged Data Payload](#tagged-data-payload)
  * [Transaction](#transaction-payload)
* [Context Inputs](#context-inputs)
  * [Commitment Input](#commitment-input)
  * [BIC Input](#block-issuance-credits-bic-input)
  * [Reward Input](#reward-input)
* [Outputs](#outputs)
  * [Basic Output](#basic-output)
  * [Foundry Output](#foundry-output)
  * [NFT Output](#nft-output)
  * [Account Output](#account-output)
* [Features](#features)
  * [Tag Feature](#tag-feature)
  * [Sender Feature](#sender-feature)
  * [Issuer Feature](#issuer-feature)
  * [Metadata Feature](#metadata-feature)
  * [Block Issuer Feature](#block-issuer-feature)
  * [Staking Feature](#staking-feature)
* [Address](#address)
  * [Ed25519 Address](#ed25519-address)
  * [Account Address](#account-address)
  * [NFT Address](#nft-address)
* [Unlock Condition](#unlock-condition)
  * [Address Unlock Condition](#address-unlock-condition)
  * [Expiration Unlock Condition](#expiration-unlock-condition)
  * [Timelock Unlock Condition](#timelock-unlock-condition)
  * [Immutable Account Address Unlock Condition](#immutable-account-address-unlock-condition)
  * [Storage Deposit Return Unlock Condition](#storage-deposit-return-unlock-condition)
  * [Governor Address Unlock Condition](#governor-address-unlock-condition)
  * [State Controller Address Unlock Condition](#state-controller-address-unlock-condition)
* [Unlocks](#unlocks)
    * [Signature Unlock](#signature-unlock)
    * [Reference Unlock](#reference-unlock)
    * [Account Unlock](#account-unlock)
    * [NFT Unlock](#nft-unlock)
* [Signature](#signature)
  * [Ed25519 Signature](#ed25519-signature)

## Block
The following table describes the serialization of a Block:

<table>
  <tr>
    <th>Name</th>
    <th>Type</th>
    <th>Description</th>
  </tr>
  <tr>
    <td>Protocol Version</td>
    <td>uint8</td>
    <td>This field denotes what protocol rules apply to the block. 
</td>
  </tr>
  <tr>
    <td>Network ID</td>
    <td>uint64</td>
    <td>Network identifier. Usually, it will be set to the first 8 bytes of the BLAKE2b-256 hash of the concatenation of the network type and the protocol version string.</td>
  </tr>
  <tr>
    <td>Strong Parents Count</td>
    <td>uint8</td>
    <td>The number of blocks that are strongly directly approved.</td>
  </tr>
  <tr>
    <td valign="top">Strong Parents <code>anyOf</code></td>
    <td colspan="2">
      <details>
        <summary>Parent</summary>
        <blockquote>
          References another directly approved block.
        </blockquote>
        <table>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
          </tr>
          <tr>
            <td>Block ID</td>
            <td>ByteArray[40]</td>
            <td>The Block ID of the strong parent.</td>
          </tr>
        </table>
      </details>
    </td>
  </tr>
  <tr>
  <tr>
    <td>Weak Parents Count</td>
    <td>uint8</td>
    <td>The number of blocks that are weakly directly approved.</td>
  </tr>
  <tr>
    <td valign="top">Weak Parents <code>anyOf</code></td>
    <td colspan="2">
      <details>
        <summary>Parent</summary>
        <blockquote>
          References another directly approved block.
        </blockquote>
        <table>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
          </tr>
          <tr>
            <td>Block ID</td>
            <td>ByteArray[40]</td>
            <td>The Block ID of the parent.</td>
          </tr>
        </table>
      </details>
    </td>
  </tr>
  <tr>
  <tr>
    <td>Shallow Like Parents Count</td>
    <td>uint8</td>
    <td>The number of blocks that are directly referenced to adjust opinion.</td>
  </tr>
  <tr>
    <td valign="top">Shallow Like Parents <code>anyOf</code></td>
    <td colspan="2">
      <details>
        <summary>Parent</summary>
        <table>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
          </tr>
          <tr>
            <td>Block ID</td>
            <td>ByteArray[40]</td>
            <td>The Block ID of the parent.</td>
          </tr>
        </table>
      </details>
    </td>
  </tr>
  <tr>
    <td>Issuer ID</td>
    <td>ByteArray[32]</td>
    <td>The issuer identifier</td>
  </tr>
  <tr>
    <td>Issuing Time</td>
    <td>uint64</td>
    <td>The time the block is issued. It's a Unix-like timestamp in nanosecond.</td>
  </tr>
  <tr>
    <td valign="top">Slot Commitment</td>
    <td colspan="2">
      <details open="true">
        <summary>Slot Commitment</summary>
        <blockquote>
          Slot Commitment is an object that contains a summary of a slot. More descriptions are in Slot Commitment section.
        </blockquote>
      </details>
    </td>
  </tr>
  <tr>
    <td>Latest Finalized Slot</td>
    <td>uint64</td>
    <td>The slot index of latest finalized slot.</td>
  </tr>
  <tr>
    <td>Payload Length</td>
    <td>uint32</td>
    <td>The length of the following payload in bytes. A length of 0 means no payload will be attached.</td>
  </tr>
  <tr>
    <td valign="top">Payload <code>optOneOf</code></td>
    <td colspan="2">
      <details>
        <summary>Tagged Data Payload</summary>
        <blockquote>
          With Payload Type 5, more details are described in Tagged Data Payload section.
        </blockquote>
      </details>
      <details>
        <summary>Transaction Payload</summary>
        <blockquote>
          With Payload Type 6, more details are described in Transaction section.
        </blockquote>
      </details>
  </tr>
  <tr>
    <td>Burned Mana</td>
    <td>uint64</td>
    <td>The amount of mana burned in this block.</td>
  </tr>
  <tr>
    <td valign="top">Signature <code>oneOf</code></td>
    <td colspan="2">
      <details>
        <summary>Ed25519 Signature</summary>
        <blockquote>
          With Signature Type 0, more details are described in Signature section.
        </blockquote>
      </details>
    </td>
    </td>
  </tr>
  <tr>
    <td>Nonce</td>
    <td>uint64</td>
    <td>The nonce which lets this block fulfill the PoW requirement.</td>
  </tr>
</table>

### Block Signature
This is how the signature is generated:

* `content` is the serialized block **without** signature and nonce.
* `slot_commitment_ID` is the identifier calculated from the Slot Commitment within the block, following steps described in [Slot Commitment ID](#slot-commitment-id) section.
* `Issuing Time` is the block issuing time.

Calculation:
1. `content_hash` = hash(`content`)
2. `data` = Concat(`Issuing Time`, `slot_commitment_ID`, `content_hash`)
3. And sign with the Issuer PubKey, `Signature` = Sign(`data`) 

Including `Issuing Time` explicitly in the signature makes the signature verification work exactly the same as a block's. So one can prove that the block existed, the content and Slot Commitment are correct, and the Issuing Time is valid.

### Block ID
Block ID denotes an identifier of a block, with type `ArrayBytes[40]`. It is calculated as the following steps, using BLAKE2b-256 hash function:

* `content` is the serialized block **without** signature and nonce.
* `slot_index` is the slot index of the `Issuing Time` of the block.
   **Note**: It's **not** the same slot index as in the Slot Commitment.
* `signatureBytes` is the serialized signature.

Calculation:
1. `content_hash` = hash(`content`)
2. `id` = hash(Concat(`content_hash`, signatureBytes, `nonce`))
3. And finally, `BlockID` = Concat(`id`, `slot_index`) 

The way of constructing Block ID allows ones to derive it without knowing the entire Block but from `content_hash`, signature, and `nonce` only. This is essential for attetstation verification (to check if other sees a valid Block with correct Block ID) in the protocol.

The string format of Block ID is hexadecimal encoding of Block ID with `0x` prefix.

### Block Syntactic Validation Rules

* Block size must not exceeds `MaxBlockSize` bytes, currently is `32768`.
* `Protocol Version` must match the `Protocol Version` config parameter of the node.
* It must hold true that 1 ≤ `Strong Parents Count` ≤ 8.
* It must hold true that 0 ≤ `Weak Parents Count` ≤ 8.
* It must hold true that 0 ≤ `Shallow Like Parents Count` ≤ 8.
* `Strong Parents`, `Weak Parents`, `Shallow Like Parents` must be lexically ordered.
* `Strong Parents`, `Weak Parents`, `Shallow Like Parents` must not have duplicates in each list.
* `Weak Parents` must be disjunct to the rest of the parents, no weak parent should be in either strong or shallow like parents.
* `Payload Type` must match one of the values described under Payloads.
  Data Fields must be correctly parsable in the context of the `Payload Type`.
  The payload itself must pass syntactic validation.
* `Signature` must be valid.
* There must be no trailing bytes after all block fields have been parsed.



## Slot Index

Timeline is divided into slots, and each slot has a corresponding slot index, which is a `uint64`.
To calculate the slot index of a timestamp, `genesisTimestamp` and the duration of a slot are needed.
The slot index of timestamp `ts` is `(ts - genesisTimestamp)/duration + 1`.

## Slot Commitment

Slot Commitment is an object that contains a summary of a slot, and it is linked to the commitment of the previous slot, which forms a commitment chain. 

**Note**: Slot Commitment and Commitment may be used interchangeably in this file and other API files. 

Multiple sparse merkle trees are managed per slot, which are:
* *Tangle tree*: All accepted blocks within a slot.
* *State mutation tree*: All accepted transactions within a slot.
* *Activity tree*: All active accounts within a slot.
* *State tree*: All accepted created/consumed outputs within a slot.
* *Mana tree*: Mana of all accounts at the end of a slot.

A slot is committable after waiting `MinCommittableSlotAge` slots from the current one. When a slot is committable, new commitment is generated and will be included in the new issued blocks. so the slot index in a slot commitment is always lagged behind of the block's slot index, which is its `Issuing Time`.

**Note** that more trees will be added in the future, but this will only change the calculation of `Root ID`, the Slot Commitment structure stays the same.

The following table describes the serialization of a Slot Commitment:

<table>
  <tr>
    <th>Name</th>
    <th>Type</th>
    <th>Description</th>
  </tr>
  <tr>
    <td>Index</td>
    <td>uint64</td>
    <td>
      The slot index of this commitment. It is calculated based on genesis timestamp and the duration of a slot.
    </td>
  </tr>
  <tr>
    <td>Previous Slot Commitment ID</td>
    <td>ByteArray[40]</td>
    <td>The commitment ID of the previous slot.</td>
  </tr>
  <tr>
    <td>Roots ID</td>
    <td>ByteArray[32]</td>
    <td>
      A BLAKE2b-256 hash of concatenating multiple sparse merkle tree roots of a slot.
      <i>Root ID = hash( Concat( hash(Concat(Tangle, TX)), hash(Concat(State, Mana)) ) )</i>  
    </td>
  </tr>
  <tr>
    <td>Cumulative Weight</td>
    <td>uint64</td>
    <td>The sum of previous slot commitment cumulative weight and weight of issuers of accepted blocks within this slot. It is just an indication of "committed into" this slot, and can not strictly be used for evaluating the switching of a chain.</td>
  </tr>
</table>

### Slot Commitment ID

Slot Commitment ID denotes an identifier of a slot commitment, with type `ArrayBytes[40]`. It is calculated as the following steps, using BLAKE2b-256 hash function:

* `content` is the serialized slot commitment.
* `slot_index` is the slot index of the slot commitment.

Calculation:
1. `content_hash` = hash(`content`)
2. And finally, `CommitmentID` = Concat(`content_hash`, `slot_index`) 

The string format of Slot Commitment ID is hexadecimal encoding of Slot Commitment ID with `0x` prefix.


## Payloads
The following table lists all currently specified payloads that can be part of a block. 

<table>
    <tr>
        <th>Name</th>
        <th>Payload Type</th>
        <th>Description</th>
    </tr>
    <tr>
        <td>Tagged Data</td>
        <td>5</td>
        <td>A payload which holds a tag and associated data.</td>
    </tr>
    <tr>
        <td>Transaction</td>
        <td>6</td>
        <td>A payload which holds a transaction with its inputs, outputs and unlocks.</td>
    </tr>
</table>

### Tagged Data payload

Tagged Data payload holds a tag and associated data.
The following table describes the serialization of a Tagged payload:

<table>
    <tr>
        <th>Name</th>
        <th>Type</th>
        <th>Description</th>
    </tr>
    <tr>
        <td>Payload Type</td>
        <td>uint32</td>
        <td>The payload type of Tagged Data payload is 5.</td>
    </tr>
    <tr>
        <td>Tag</td>
        <td>(uint8)ByteArray</td>
        <td>The tag to use to categorize the data in binary form. A leading uint8 denotes its length.</td>
    </tr>
    <tr>
        <td>Data</td>
        <td>(uint32)ByteArray</td>
        <td>Binary data. A leading uint32 denotes its length.</td>
    </tr>
</table>

#### Tagged Payload Syntactic Validation Rules
* The length of a Tag must < `64` bytes.
* The length of the Data must < `8192` bytes.

### Transaction Payload    

The following table describes the serialization of a Transaction Payload:

<table>
  <tr>
    <th>Name</th>
    <th>Type</th>
    <th>Description</th>
  </tr>
  <tr>
        <td>Payload Type</td>
        <td>uint32</td>
        <td>The payload type of Transaction payload is 6.</td>
    </tr>
  <tr>
    <td valign="top">Essence <code>oneOf</code></td>
    <td colspan="2">
      <details open="true">
        <summary>Transaction Essence</summary>
        <blockquote>
          Describes the essence data making up a transaction by defining its inputs, outputs and an optional payload.
        </blockquote>
        <table>
          <tr>
            <td><b>Name</b></td>
            <td><b>Type</b></td>
            <td><b>Description</b></td>
          </tr>
          <tr>
            <td>Transaction Essence Type</td>
            <td>uint8</td>
            <td>
              Set to value 2. 
            </td>
          </tr>
          <tr>
            <td>Network ID</td>
            <td>uint64</td>
            <td>
              The unique value denoting whether the block was meant for mainnet, shimmer, testnet, or a private network. It consists of the first 8 bytes of the BLAKE2b-256 hash of the network name.
            </td>
          </tr>
          <tr>
            <td>Creation Time</td>
            <td>uint64</td>
            <td>The <b>slot index</b> at which this transaction was created by the client.</td>
          </tr>
          <tr>
            <td>Context Inputs Count</td>
            <td>uint16</td>
            <td>The number of context input entries.</td>
          </tr>
          <tr>
            <td valign="top">Context Inputs <code>anyOf</code></td>
            <td colspan="2">
              <details>
                <summary>Commitment Input</summary>
                <blockquote>
                  Describes an input which references commitment to a certain slot.
                </blockquote>
              </details>
              <details>
                <summary>BIC Input</summary>
                <blockquote>
                  Describes an input which denotes an BIC value of an account.
                </blockquote>
              </details>
              <details>
                <summary>Reward Input</summary>
                <blockquote>
                  Describes an input which claims the reward.
                </blockquote>
              </details>
            </td>
          </tr>
          <tr>
          <tr>
            <td>Inputs Count</td>
            <td>uint16</td>
            <td>The number of input entries.</td>
          </tr>
          <tr>
            <td valign="top">Inputs <code>anyOf</code></td>
            <td colspan="2">
              <details>
                <summary>UTXO Input</summary>
                <blockquote>
                  Describes an input which references an unspent transaction output to consume.
                </blockquote>
                <table>
                  <tr>
                    <td><b>Name</b></td>
                    <td><b>Type</b></td>
                    <td><b>Description</b></td>
                  </tr>
                  <tr>
                    <td>Input Type</td>
                    <td>uint8</td>
                    <td>
                      Set to <strong>value 0</strong> to denote an <i>TIP-20 UTXO Input</i>.
                    </td>
                  </tr>
                  <tr>
                    <td>Transaction ID</td>
                    <td>ByteArray[32]</td>
                    <td>The BLAKE2b-256 hash of the transaction payload containing the referenced output.</td>
                  </tr>
                  <tr>
                    <td>Transaction Output Index</td>
                    <td>uint16</td>
                    <td>The output index of the referenced output.</td>
                  </tr>
                </table>
              </details>
            </td>
          </tr>
          <tr>
            <td>Inputs Commitment</td>
            <td>ByteArray[32]</td>
            <td>
              BLAKE2b-256 hash serving as a commitment to the serialized outputs referenced by <code>Inputs</code>.
            </td>
          </tr>
          <tr>
            <td>Outputs Count</td>
            <td>uint16</td>
            <td>The number of output entries.</td>
          </tr>
          <tr>
            <td valign="top">Outputs <code>anyOf</code></td>
            <td colspan="2">
              <details>
                <summary>Basic Output</summary>
                <blockquote>
                  Describes a deposit to a single address. The output might contain optional features and native tokens.
                </blockquote>
              </details>
              <details>
                <summary>Account Output</summary>
                <blockquote>
                  Describes an account in the ledger.
                </blockquote>
              </details>
              <details>
                <summary>Foundry Output</summary>
                <blockquote>
                  Describes a foundry that controls supply of native tokens.
                </blockquote>
              </details>
              <details>
                <summary>NFT Output</summary>
                <blockquote>
                  Describes a unique, non-fungible token deposit to a single address.
                </blockquote>
              </details>
            </td>
          </tr>
          <tr>
            <td>Allotment Count</td>
            <td>uint16</td>
            <td>The number of allotment entries.</td>
          </tr>
          <tr>
            <td valign="top">Allotments <code>anyOf</code></td>
            <td colspan="2">
              <details>
                <summary>Allotment</summary>
                <blockquote>
                  Describes a list of accounts with the amount of mana to allot to.
                </blockquote>
                <table>
                  <tr>
                    <td><b>Name</b></td>
                    <td><b>Type</b></td>
                    <td><b>Description</b></td>
                  </tr>
                  <tr>
                    <td>Account ID</td>
                    <td>ByteArray[32]</td>
                    <td>
                      The account identifier to allot to.
                    </td>
                  </tr>
                  <tr>
                    <td>Value</td>
                    <td>uint64</td>
                    <td>The amount of mana to allot.</td>
                  </tr>
                </table>
              </details>              
            </td>
          </tr>
          <tr>
            <td>Payload Length</td>
            <td>uint32</td>
            <td>The length in bytes of the optional payload.</td>
          </tr>
          <tr>
            <td valign="top">Payload <code>optOneOf</code></td>
            <td colspan="2">
              <details>
                <summary>Tagged Data Payload</summary>
                <blockquote>
                  Describes data with optional tag, defined in previous section.
                </blockquote>
              </details>
            </td>
          </tr>
        </table>
      </details>
    </td>
  </tr>
  <tr>
    <td>Unlocks Count</td>
    <td>uint16</td>
    <td>The number of unlock entries. It must match the field <code>Inputs Count</code>.</td>
  </tr>
  <tr>
    <td valign="top">Unlocks <code>anyOf</code></td>
    <td colspan="2">
      <details>
        <summary>Signature Unlock</summary>
        <blockquote>
          Defines an unlock containing a signature.
        </blockquote>
      </details>
      <details>
        <summary>Reference Unlock</summary>
        <blockquote>
          References a previous unlock, where the same unlock can be used for multiple inputs.
        </blockquote>
      </details>
      <details>
        <summary>Account Unlock</summary>
        <blockquote>
          References a previous unlock of a consumed account output.
        </blockquote>
      </details>
      <details>
        <summary>NFT Unlock</summary>
        <blockquote>
          References a previous unlock of a consumed NFT output.
        </blockquote>
      </details>
    </td>
  </tr>
</table>

#### Transaction Validation Rules
Transaction validation rules follow [TIP 20 Transaction Payload with TIP-18 Output Types](https://github.com/iotaledger/tips/blob/ae7fbd336e506f907080aa6b16d6ffb0d7a5553c/tips/TIP-0020/tip-0020.md) with additional rules for new fields:

* 0 < `Context Inputs Count` < 128
* No duplicate element of `Context Inputs` is allowed.


## Context Inputs

Context Inputs are inputs that indicate the different functions related to account, commitment and more. There could be more types of context inputs introduced in the future via protocol upgrades.

Each input must be accompanied by a corresponding Unlock at the same index in the `Unlocks` part of the Transaction Payload.

The following table lists all currently specified Context Input.

<table>
    <tr>
        <th>Name</th>
        <th>Context Input Type</th>
        <th>Description</th>
    </tr>
    <tr>
        <td>Commitment Input</td>
        <td>1</td>
        <td>An input which allows to reference commitment to a certain slot.</td>
    </tr>
    <tr>
        <td>Block Issuance Credits (BIC) Input</td>
        <td>2</td>
        <td>An input which allows to provide the VM with context for the value of the BIC vector for a specific slot.</td>
    </tr>
    <tr>
        <td>Reward Input</td>
        <td>3</td>
        <td>An input which Allows to provide the VM with context of claiming rewards.</td>
    </tr>
</table>

### Commitment Input

A Commitment Input is an input that allows to reference commitment to a certain slot. It is used to provide the VM with the necessary context information from the node, to prove that the time at the transaction execution is past certain slot in the past, as it indicates that the slot has been already committed.

<table>
    <tr>
        <th>Name</th>
        <th>Type</th>
        <th>Description</th>
    </tr>
    <tr>
        <td>Input Type</td>
        <td>uint16</td>
        <td>The input type of Commitment Input is 1.</td>
    </tr>
    <tr>
        <td>Commitment ID</td>
        <td>ByteArray[40]</td>
        <td>The commitment identifier to reference to.</td>
    </tr>
</table>

### Block Issuance Credits (BIC) Input

A Block Issuance Credits Input is an input that allows to provide the VM with context for the value of the BIC vector for a specific slot. It is necessary information needed for any Account transitions, and account destroying. 

<table>
    <tr>
        <th>Name</th>
        <th>Type</th>
        <th>Description</th>
    </tr>
    <tr>
        <td>Input Type</td>
        <td>uint16</td>
        <td>The input type of BIC Input is 2.</td>
    </tr>
    <tr>
        <td>Account ID</td>
        <td>ByteArray[32]</td>
        <td>The BIC of an account to use.</td>
    </tr>
</table>

### Reward Input

A Reward Input is an input indicates which transaction `Input` is the claiming reward. 

<table>
    <tr>
        <th>Name</th>
        <th>Type</th>
        <th>Description</th>
    </tr>
    <tr>
        <td>Input Type</td>
        <td>uint16</td>
        <td>The input type of Rewarad Input is 3.</td>
    </tr>
    <tr>
        <td>Index</td>
        <td>uint16</td>
        <td>The index of the transaction input for which to claim rewards.</td>
    </tr>
</table>




## Outputs
The following table lists all currently specified Output.

<table>
    <tr>
        <th>Name</th>
        <th>Output Type</th>
    </tr>
    <tr>
        <td>Basic Output</td>
        <td>3</td>
    </tr>
    <tr>
        <td>Account Output</td>
        <td>4</td>
    </tr>
    <tr>
        <td>Foundry Output</td>
        <td>5</td>
    </tr>
    <tr>
        <td>NFT Output</td>
        <td>6</td>
    </tr>
</table>

### Basic Output
A Basic Output can hold native tokens and might have several unlock conditions and optional features. The combination of several features provide the base functionality for the output to be used as an on-ledger smart contract request.

<table>
  <tr>
      <td><b>Name</b></td>
      <td><b>Type</b></td>
      <td><b>Description</b></td>
  </tr>
  <tr>
      <td>Output Type</td>
      <td>uint8</td>
      <td>
          Set to <strong>value 3</strong> to denote a <i>Basic Output</i>.
      </td>
  </tr>
  <tr>
      <td>Amount</td>
      <td>uint64</td>
      <td>The amount of IOTA coins held by the output.</td>
  </tr>
  <tr>
      <td>Native Tokens Count</td>
      <td>uint8</td>
      <td>The number of native tokens held by the output.</td>
  </tr>
  <tr>
      <td valign="top">Native Tokens <code>optAnyOf</code></td>
      <td colspan="2">
          <details>
              <summary>Native Token</summary>
              <table>
                  <tr>
                      <td><b>Name</b></td>
                      <td><b>Type</b></td>
                      <td><b>Description</b></td>
                  </tr>
                  <tr>
                      <td>Token ID</td>
                      <td>ByteArray[38]</td>
                      <td>
                          Identifier of the native token.
                      </td>
                  </tr>
                  <tr>
                      <td>Amount</td>
                      <td>uint256</td>
                      <td>
                          Amount of native tokens of the given <i>Token ID</i>.
                      </td>
                  </tr>
              </table>
          </details>
      </td>
  </tr>
  <tr>
      <td>Unlock Conditions Count</td>
      <td>uint8</td>
      <td>The number of unlock conditions following.</td>
  </tr>
  <tr>
      <td valign="top">Unlock Conditions <code>atMostOneOfEach</code></td>
      <td colspan="2">
          <details>
              <summary>Address Unlock Condition</summary>
          </details>
          <details>
              <summary>Storage Deposit Return Unlock Condition</summary>
              <blockquote>
                  Defines the amount of IOTAs used as storage deposit that have to be returned to <i>Return Address</i>.
              </blockquote>
          </details>
          <details>
              <summary>Timelock Unlock Condition</summary>
              <blockquote>
                  Defines a slot index until which the output can not be unlocked.
              </blockquote>
          </details>
          <details>
              <summary>Expiration Unlock Condition</summary>
              <blockquote>
                  Defines a slot index until which only <i>Address</i>, defined in <i>Address Unlock Condition</i>,
                  is allowed to unlock the output. After the slot index is reached/passed, only
                  <i>Return Address</i> can unlock it.
              </blockquote>
          </details>
      </td>
  </tr>
  <tr>
      <td>Features Count</td>
      <td>uint8</td>
      <td>The number of features following.</td>
  </tr>
  <tr>
      <td valign="top">Features <code>atMostOneOfEach</code></td>
      <td colspan="2">
          <details>
              <summary>Sender Feature</summary>
              <blockquote>
                  Identifies the validated sender of the output.
              </blockquote>
          </details>
          <details>
              <summary>Metadata Feature</summary>
              <blockquote>
                  Defines metadata (arbitrary binary data) that will be stored in the output.
              </blockquote>
          </details>
          <details>
              <summary>Tag Feature</summary>
              <blockquote>
                  Defines an indexation tag to which the output can be indexed by additional node plugins.
              </blockquote>
          </details>
      </td>
  </tr>
  <tr>
      <td>Mana</td>
      <td>uint64</td>
      <td>The amount of (stored) Mana held by the output.</td>
  </tr>
</table>

### Foundry Output
A foundry output is an output that controls the supply of user defined native tokens. It can mint and melt tokens according to the policy defined in the Token Scheme field of the output. Foundries can only be created and controlled by accounts.

<table>
  <tr>
      <td><b>Name</b></td>
      <td><b>Type</b></td>
      <td><b>Description</b></td>
  </tr>
  <tr>
      <td>Output Type</td>
      <td>uint8</td>
      <td>
          Set to <strong>value 5</strong> to denote a <i>Foundry Output</i>.
      </td>
  </tr>
  <tr>
      <td>Amount</td>
      <td>uint64</td>
      <td>The amount of IOTA coins held by the output.</td>
  </tr>
  <tr>
      <td>Native Tokens Count</td>
      <td>uint8</td>
      <td>The number of different native tokens held by the output.</td>
  </tr>
  <tr>
      <td valign="top">Native Tokens <code>optAnyOf</code></td>
      <td colspan="2">
          <details>
              <summary>Native Token</summary>
              <table>
                  <tr>
                      <td><b>Name</b></td>
                      <td><b>Type</b></td>
                      <td><b>Description</b></td>
                  </tr>
                  <tr>
                      <td>Token ID</td>
                      <td>ByteArray[38]</td>
                      <td>
                          Identifier of the native tokens.
                      </td>
                  </tr>
                  <tr>
                      <td>Amount</td>
                      <td>uint256</td>
                      <td>Amount of native tokens of the given <i>Token ID</i>.</td>
                  </tr>
              </table>
          </details>
      </td>
  </tr>
  <tr>
      <td>Serial Number</td>
      <td>uint32</td>
      <td>The serial number of the foundry with respect to the controlling account.</td>
  </tr>
  <tr>
      <td valign="top">Token Scheme <code>oneOf</code></td>
      <td colspan="2">
          <details>
              <summary>Simple Token Scheme</summary>
              <table>
                  <tr>
                      <td><b>Name</b></td>
                      <td><b>Type</b></td>
                      <td><b>Description</b></td>
                  </tr>
                  <tr>
                      <td>Token Scheme Type</td>
                      <td>uint8</td>
                      <td>
                          Set to <strong>value 0</strong> to denote an <i>Simple Token Scheme</i>.
                      </td>
                  </tr>
                  <tr>
                      <td>Minted Tokens</td>
                      <td>uint256</td>
                      <td>Amount of tokens minted by this foundry.</td>
                  </tr>
                  <tr>
                      <td>Melted Tokens</td>
                      <td>uint256</td>
                      <td>Amount of tokens melted by this foundry.</td>
                  </tr>
                  <tr>
                      <td>Maximum Supply</td>
                      <td>uint256</td>
                      <td>Maximum supply of tokens controlled by this foundry.</td>
                  </tr>
              </table>
          </details>
      </td>
  </tr>
  <tr>
      <td>Unlock Conditions Count</td>
      <td>uint8</td>
      <td>The number of unlock conditions following.</td>
  </tr>
  <tr>
      <td valign="top">Unlock Conditions <code>atMostOneOfEach</code></td>
      <td colspan="2">
          <details>
              <summary>Immutable Account Address Unlock Condition</summary>
          </details>
      </td>
  </tr>
  <tr>
      <td>Features Count</td>
      <td>uint8</td>
      <td>The number of features following.</td>
  </tr>
  <tr>
      <td valign="top">Features <code>atMostOneOfEach</code></td>
      <td colspan="2">
          <details>
              <summary>Metadata Feature</summary>
              <blockquote>
                  Defines metadata (arbitrary binary data) that will be stored in the output.
              </blockquote>
          </details>
      </td>
  </tr>
  <tr>
      <td>Immutable Features Count</td>
      <td>uint8</td>
      <td>The number of immutable features following. Immutable features are defined upon deployment of the UTXO state machine and are not allowed to change in any future state transition.</td>
  </tr>
  <tr>
      <td valign="top">Immutable Features <code>atMostOneOfEach</code></td>
      <td colspan="2">
          <details>
              <summary>Metadata Feature</summary>
              <blockquote>
                  Defines metadata (arbitrary binary data) that will be stored in the output.
              </blockquote>
          </details>
      </td>
  </tr>
  <tr>
      <td>Mana</td>
      <td>uint64</td>
      <td>The amount of stored mana held by the outptu.</td>
  </tr>
</table>

### NFT Output

<table>
  <tr>
      <td><b>Name</b></td>
      <td><b>Type</b></td>
      <td><b>Description</b></td>
  </tr>
  <tr>
      <td>Output Type</td>
      <td>uint8</td>
      <td>
          Set to <strong>value 6</strong> to denote a <i>NFT Output</i>.
      </td>
  </tr>
  <tr>
      <td>Amount</td>
      <td>uint64</td>
      <td>The amount of IOTA coins held by the output.</td>
  </tr>  
  <tr>
      <td>Native Tokens Count</td>
      <td>uint8</td>
      <td>The number of native tokens held by the output.</td>
  </tr>
  <tr>
      <td valign="top">Native Tokens <code>optAnyOf</code></td>
      <td colspan="2">
          <details>
              <summary>Native Token</summary>
              <table>
                  <tr>
                      <td><b>Name</b></td>
                      <td><b>Type</b></td>
                      <td><b>Description</b></td>
                  </tr>
                  <tr>
                      <td>Token ID</td>
                      <td>ByteArray[38]</td>
                      <td>
                          Identifier of the native token.
                      </td>
                  </tr>
                  <tr>
                      <td>Amount</td>
                      <td>uint256</td>
                      <td>
                          Amount of native tokens of the given <i>Token ID</i>.
                      </td>
                  </tr>
              </table>
          </details>
      </td>
  </tr>
  <tr>
      <td>NFT ID</td>
      <td>ByteArray[32]</td>
      <td>Unique identifier of the NFT, which is the BLAKE2b-256 hash of the <i>Output ID</i> that created it.<i> NFT Address = NFT Address Type || NFT ID</i></td>
  </tr>
  <tr>
      <td>Unlock Conditions Count</td>
      <td>uint8</td>
      <td>The number of unlock conditions following.</td>
  </tr>
  <tr>
      <td valign="top">Unlock Conditions <code>atMostOneOfEach</code></td>
      <td colspan="2">
          <details>
              <summary>Address Unlock Condition</summary>
          </details>
          <details>
              <summary>Storage Deposit Return Unlock Condition</summary>
              <blockquote>
                  Defines the amount of IOTAs used as storage deposit that have to be returned to <i>Return Address</i>.
              </blockquote>
          </details>
          <details>
              <summary>Timelock Unlock Condition</summary>
              <blockquote>
                  Defines a slot index until which the output can not be unlocked.
              </blockquote>
          </details>
          <details>
              <summary>Expiration Unlock Condition</summary>
              <blockquote>
                  Defines a slot index until which only <i>Address</i>, defined in <i>Address Unlock Condition</i>,
                  is allowed to unlock the output. After the slot index is reached/passed, only
                  <i>Return Address</i> can unlock it.
              </blockquote>
          </details>
      </td>
  </tr>
  <tr>
      <td>Features Count</td>
      <td>uint8</td>
      <td>The number of features following.</td>
  </tr>
  <tr>
      <td valign="top">Features <code>optAnyOf</code></td>
      <td colspan="2">
          <details>
              <summary>Sender Feature</summary>
              <blockquote>
                  Identifies the validated sender of the output.
              </blockquote>
          </details>
          <details>
              <summary>Metadata Feature</summary>
              <blockquote>
                  Defines metadata (arbitrary binary data) that will be stored in the output.
              </blockquote>
          </details>
          <details>
              <summary>Tag Feature</summary>
              <blockquote>
                  Defines an indexation tag to which the output can be indexed by additional node plugins.
              </blockquote>
          </details>
      </td>
  </tr>
  <tr>
      <td>Immutable Features Count</td>
      <td>uint8</td>
      <td>The number of immutable features following. Immutable features are defined upon deployment of the UTXO state machine and are not allowed to change in any future state transition.</td>
  </tr>
  <tr>
      <td valign="top">Immutable Features <code>optAnyOf</code></td>
      <td colspan="2">
          <details>
              <summary>Issuer Feature</summary>
              <blockquote>
                  Identifies the validated issuer of the UTXO state machine.
              </blockquote>
          </details>
          <details>
              <summary>Metadata Feature</summary>
              <blockquote>
                  Defines metadata (arbitrary binary data) that will be stored in the output.
              </blockquote>
          </details>
      </td>
  </tr>
  <tr>
      <td>Mana</td>
      <td>uint64</td>
      <td>The amount of stored Mana held by the output.</td>
  </tr>
</table>


### Account Output

<table>
  <tr>
      <td><b>Name</b></td>
      <td><b>Type</b></td>
      <td><b>Description</b></td>
  </tr>
  <tr>
      <td>Output Type</td>
      <td>uint8</td>
      <td>
          Set to <strong>value 4</strong> to denote a <i>Account Output</i>.
      </td>
  </tr>
  <tr>
      <td>Amount</td>
      <td>uint64</td>
      <td>The amount of IOTA coins held by the output.</td>
  </tr>
  <tr>
      <td>Native Tokens Count</td>
      <td>uint8</td>
      <td>The number of native tokens held by the output.</td>
  </tr>
  <tr>
      <td valign="top">Native Tokens <code>optAnyOf</code></td>
      <td colspan="2">
          <details>
              <summary>Native Token</summary>
              <table>
                  <tr>
                      <td><b>Name</b></td>
                      <td><b>Type</b></td>
                      <td><b>Description</b></td>
                  </tr>
                  <tr>
                      <td>Token ID</td>
                      <td>ByteArray[38]</td>
                      <td>
                          Identifier of the native token.
                      </td>
                  </tr>
                  <tr>
                      <td>Amount</td>
                      <td>uint256</td>
                      <td>
                          Amount of native tokens of the given <i>Token ID</i>.
                      </td>
                  </tr>
              </table>
          </details>
      </td>
  </tr>
  <tr>
      <td>Account ID</td>
      <td>ByteArray[32]</td>
      <td>Unique identifier of the account, which is the BLAKE2b-256 hash of the <i>Output ID</i> that created it.<i> Account Address = Account Address Type || Account ID</i></td>
  </tr>
  <tr>
      <td>State Index</td>
      <td>uint32</td>
      <td>A counter that must increase by 1 every time the account is state transitioned.</td>
  </tr>
  <tr>
      <td>State Metadata</td>
      <td>(uint16)ByteArray</td>
      <td>Metadata that can only be changed by the state controller. A leading uint16 denotes its length.</td>
  </tr>
  <tr>
      <td>Foundry Counter</td>
      <td>uint32</td>
      <td>A counter that denotes the number of foundries created by this account.</td>
  </tr>
  <tr>
      <td>Unlock Conditions Count</td>
      <td>uint8</td>
      <td>The number of unlock conditions following.</td>
  </tr>
  <tr>
      <td valign="top">Unlock Conditions <code>atMostOneOfEach</code></td>
      <td colspan="2">
          <details>
              <summary>State Controller Address Unlock Condition</summary>
          </details>
          <details>
              <summary>Governor Address Unlock Condition</summary>
          </details>
      </td>
  </tr>
  <tr>
      <td>Features Count</td>
      <td>uint8</td>
      <td>The number of features following.</td>
  </tr>
  <tr>
      <td valign="top">Features <code>atMostOneOfEach</code></td>
      <td colspan="2">
          <details>
              <summary>Sender Feature</summary>
              <blockquote>
                  Identifies the validated sender of the output.
              </blockquote>
          </details>
          <details>
              <summary>Metadata Feature</summary>
              <blockquote>
                  Defines metadata (arbitrary binary data) that will be stored in the output.
              </blockquote>
          </details>
          <details>
              <summary>Block Issuer Feature</summary>
              <blockquote>
                  Contains the public keys to verify block signatures and allows for unbonding the issuer deposit.
              </blockquote>
          </details>
      </td>
  </tr>
  <tr>
      <td>Immutable Features Count</td>
      <td>uint8</td>
      <td>The number of immutable features following. Immutable features are defined upon deployment of the UTXO state machine and are not allowed to change in any future state transition.</td>
  </tr>
  <tr>
      <td valign="top">Immutable Features <code>atMostOneOfEach</code></td>
      <td colspan="2">
          <details>
              <summary>Issuer Feature</summary>
              <blockquote>
                  Identifies the validated issuer of the UTXO state machine.
              </blockquote>
          </details>
          <details>
              <summary>Metadata Feature</summary>
              <blockquote>
                  Defines metadata (arbitrary binary data) that will be stored in the output.
              </blockquote>
          </details>
      </td>
  </tr>
   <tr>
      <td>Mana</td>
      <td>uint64</td>
      <td>The amount of stored Mana held by the output.</td>
  </tr>
</table>


## Features

Output Features that do not introduce unlocking conditions, but rather add new functionality and add constraints on output creation are grouped under Features. Each output must not contain more than one feature of each type and not all feature types are supported for each output type.

Details of `Sender Feature`, `Issuer Feature`, `Metadata Feature` and `Tag Feature`  are described in [TIP-18 Multi-Asset Ledger and ISC Support](https://github.com/iotaledger/tips/blob/ae7fbd336e506f907080aa6b16d6ffb0d7a5553c/tips/TIP-0018/tip-0018.md#features). This document introduces 2 additional new features, `Block Issuer Feature` and `Staking Feature`.

The following table lists all currently specified Features.

<table>
    <tr>
        <th>Name</th>
        <th>Feature Type</th>
    </tr>
    <tr>
        <td>Sender Feature</td>
        <td>0</td>
    </tr>
    <tr>
        <td>Issuer Feature</td>
        <td>1</td>
    </tr>
    <tr>
        <td>Metadata Feature</td>
        <td>2</td>
    </tr>
    <tr>
        <td>Tag Feature</td>
        <td>3</td>
    </tr>
    <tr>
        <td>Block Issuer Feature</td>
        <td>4</td>
    </tr>
    <tr>
        <td>Staking Feature</td>
        <td>5</td>
    </tr>
</table>


### Sender Feature
SenderFeature is a feature which associates an output with a sender identity.

<table>
  <tr>
      <td><b>Name</b></td>
      <td><b>Type</b></td>
      <td><b>Description</b></td>
  </tr>
  <tr>
      <td>Feature Type</td>
      <td>uint8</td>
      <td>
          Set to <strong>value 0</strong> to denote a <i>Sender Feature</i>.
      </td>
  </tr>
  <tr>
      <td valign="top">Sender <code>oneOf</code></td>
      <td colspan="2">
          <details>
              <summary>Ed25519 Address</summary>
          </details>
          <details>
              <summary>Account Address</summary>
          </details>
          <details>
              <summary>NFT Address</summary>
          </details>
      </td>
  </tr>
</table>


### Issuer Feature
Issuer Feature is a feature which associates an output with an issuer identity. 

<table>
  <tr>
      <td><b>Name</b></td>
      <td><b>Type</b></td>
      <td><b>Description</b></td>
  </tr>
  <tr>
      <td>Feature Type</td>
      <td>uint8</td>
      <td>
          Set to <strong>value 1</strong> to denote an <i>Issuer Feature</i>.
      </td>
  </tr>
  <tr>
      <td valign="top">Issuer <code>oneOf</code></td>
      <td colspan="2">
          <details>
              <summary>Ed25519 Address</summary>
          </details>
          <details>
              <summary>Account Address</summary>
          </details>
          <details>
              <summary>NFT Address</summary>
          </details>
      </td>
  </tr>
</table>

### Metadata Feature
Metadata Feature is a feature which simply holds binary data to be freely interpreted by higher layer applications.

 <table>
  <tr>
      <td><b>Name</b></td>
      <td><b>Type</b></td>
      <td><b>Description</b></td>
  </tr>
  <tr>
      <td>Feature Type</td>
      <td>uint8</td>
      <td>
          Set to <strong>value 2</strong> to denote a <i>Metadata Feature</i>.
      </td>
  </tr>
  <tr>
      <td>Data</td>
      <td>(uint16)ByteArray</td>
      <td>Binary data. A leading uint16 denotes its length.</td>
  </tr>
</table>

### Tag Feature
Tag Feature is a feature which allows to additionally tag an output by a user defined value.

<table>
    <tr>
        <td><b>Name</b></td>
        <td><b>Type</b></td>
        <td><b>Description</b></td>
    </tr>
    <tr>
        <td>Feature Type</td>
        <td>uint8</td>
        <td>
            Set to <strong>value 3</strong> to denote a <i>Tag Feature</i>.
        </td>
    </tr>
    <tr>
        <td>Tag</td>
        <td>(uint8)ByteArray</td>
        <td>Binary indexation data. A leading uint8 denotes its length.</td>
    </tr>
</table>


### Block Issuer Feature
Block Issuer Feature is a feature which indicates that this account can issue blocks.
The feature includes a block issuer address as well as an expiry slot.

<table>
    <tr>
        <th>Name</th>
        <th>Type</th>
        <th>Description</th>
    </tr>
    <tr>
        <td>Feature Type</td>
        <td>uint8</td>
        <td>The input type of Block Issuer Feature is 4.</td>
    </tr>
    <tr>
      <td>Block Issuer Keys Count</td>
      <td>uint8</td>
      <td>The number of block issuer keys entries.</td>
    </tr>
    <tr>
        <td valign="top">Block Issuer Keys <code>anyOf</code></td>
        <td colspan="2">
            <details>
                <summary>Ed25519 Public Key</summary>
                <table>
                    <tr>
                        <td><b>Name</b></td>
                        <td><b>Type</b></td>
                        <td><b>Description</b></td>
                    </tr>
                    <tr>
                        <td>Public Key Type</td>
                        <td>uint8</td>
                        <td>
                            Set to <strong>value 0</strong> to denote an <i>Ed25519 Public Key</i>.
                        </td>
                    </tr>
                    <tr>
                        <td>Public Key</td>
                        <td>ByteArray[32]</td>
                        <td>The raw bytes of the Ed25519 public key.</td>
                    </tr>
                </table>
            </details>
        </td>
    </tr>
    <tr>
        <td>Expiry Slot</td>
        <td>uint64</td>
        <td>The slot index at which the issuer deposit can be unlocked.</td>
    </tr>
</table>

### Staking Feature
Staking Feature is a feature which indicates that this account wants to register as a validator.
The feature includes a fixed cost that the staker can set and will receive as part of its rewards, as well as a range of epoch indices in which the feature is considered active and can claim rewards.

<table>
    <tr>
        <th>Name</th>
        <th>Type</th>
        <th>Description</th>
    </tr>
    <tr>
        <td>Staking Type</td>
        <td>uint8</td>
        <td>The input type of Staking Feature is 5.</td>
    </tr>
    <tr>
        <td>Staked Amount</td>
        <td>uint64</td>
        <td>The amount of IOTA coins that are locked and staked in the containing account.</td>
    </tr>
    <tr>
        <td>Fixed Cost</td>
        <td>uint64</td>
        <td>The fixed cost of the validator, which it receives as part of its Mana rewards.</td>
    </tr>
    <tr>
        <td>Start Epoch</td>
        <td>uint64</td>
        <td>The epoch index at which the staking starts.</td>
    </tr>
    <tr>
        <td>End Epoch</td>
        <td>uint64</td>
        <td>The epoch index at which the staking ends.</td>
    </tr>
</table>




## Address

The following table lists all currently specified Address.

<table>
    <tr>
        <th>Name</th>
        <th>Output Type</th>
    </tr>
    <tr>
        <td>Ed25519 Address</td>
        <td>0</td>
    </tr>
    <tr>
        <td>Account Address</td>
        <td>8</td>
    </tr>
    <tr>
        <td>NFT Address</td>
        <td>16</td>
    </tr>
</table>

### Ed25519 Address

<table>
  <tr>
      <td><b>Name</b></td>
      <td><b>Type</b></td>
      <td><b>Description</b></td>
  </tr>
  <tr>
      <td>Address Type</td>
      <td>uint8</td>
      <td>
          Set to <strong>value 0</strong> to denote an <i>Ed25519 Address</i>.
      </td>
  </tr>
  <tr>
      <td>PubKeyHash</td>
      <td>ByteArray[32]</td>
      <td>The raw bytes of the Ed25519 address which is a BLAKE2b-256 hash of the Ed25519 public key.</td>
  </tr>
</table>

### Account Address

<table>
  <tr>
      <td><b>Name</b></td>
      <td><b>Type</b></td>
      <td><b>Description</b></td>
  </tr>
  <tr>
      <td>Address Type</td>
      <td>uint8</td>
      <td>
          Set to <strong>value 8</strong> to denote an <i>Account Address</i>.
      </td>
  </tr>
  <tr>
      <td>Account ID</td>
      <td>ByteArray[32]</td>
      <td>The raw bytes of the <i>Account ID</i> which is the BLAKE2b-256 hash of the outputID that created it.</td>
  </tr>
</table>

### NFT Address
<table>
  <tr>
      <td><b>Name</b></td>
      <td><b>Type</b></td>
      <td><b>Description</b></td>
  </tr>
  <tr>
      <td>Address Type</td>
      <td>uint8</td>
      <td>
          Set to <strong>value 16</strong> to denote an <i>NFT Address</i>.
      </td>
  </tr>
  <tr>
      <td>NFT ID</td>
      <td>ByteArray[32]</td>
      <td>The raw bytes of the <i>NFT ID</i> which is the BLAKE2b-256 hash of the outputID that created it.</td>
  </tr>
</table>


## Unlock Condition

Unlock Condition designs are based on [TIP-18 Multi-Asset Ledger and ISC Support](https://github.com/iotaledger/tips/blob/main/tips/TIP-0018/tip-0018.md#output-design).

No new unlock Condition types are introduced, only a few changes are made:
* Rename `Alias` to `Account`.
* Change `Unix Time` to `Slot Index` in Timelock Unlock Condition. 
* Change `Unix Time` to `Slot Index` in Expiration Unlock Condition. 

The following table lists all currently specified Unlock Condition.

<table>
    <tr>
        <th>Name</th>
        <th>Unlock Condition Type</th>
    </tr>
    <tr>
        <td>Address Unlock Condition</td>
        <td>0</td>
    </tr>
    <tr>
        <td>Storage Deposit Return Unlock Condition</td>
        <td>1</td>
    </tr>
    <tr>
        <td>Timelock Unlock Condition</td>
        <td>2</td>
    </tr>
    <tr>
        <td>Expiration Unlock Condition</td>
        <td>3</td>
    </tr>
    <tr>
        <td>State Controller Address Unlock Condition</td>
        <td>4</td>
    </tr>
    <tr>
        <td>Governor Address Unlock Condition</td>
        <td>5</td>
    </tr>
    <tr>
        <td>Immutable Account Address Unlock Condition</td>
        <td>6</td>
    </tr>
</table>

### Address Unlock Condition

Address Unlock Condition is an Unlock Condition defining an identity which has to be unlocked.

 <table>
    <tr>
        <td><b>Name</b></td>
        <td><b>Type</b></td>
        <td><b>Description</b></td>
    </tr>
    <tr>
        <td>Unlock Condition Type</td>
        <td>uint8</td>
        <td>
            Set to <strong>value 0</strong> to denote an <i>Address Unlock Condition</i>.
        </td>
    </tr>
    <tr>
        <td>Address</td>
        <td colspan="2">
            <details>
                <summary>Ed25519 Address</summary>
            </details>
            <details>
                <summary>Account Address</summary>
            </details>
            <details>
                <summary>NFT Address</summary>
            </details>
        </td>
    </tr>
</table>

### Storage Deposit Return Unlock Condition

StorageDepositReturnUnlockCondition is an unlock condition which defines the amount of tokens which must be sent back to the return identity, when the output in which it occurs in, is consumed.

<table>
    <tr>
        <td><b>Name</b></td>
        <td><b>Type</b></td>
        <td><b>Description</b></td>
    </tr>
    <tr>
        <td>Unlock Condition Type</td>
        <td>uint8</td>
        <td>
            Set to <strong>value 1</strong> to denote a <i>Storage Deposit Return Unlock Condition</i>.
        </td>
    </tr>
    <tr>
        <td valign="top">Return Address <code>oneOf</code></td>
        <td colspan="2">
            <details>
                <summary>Ed25519 Address</summary>
            </details>
            <details>
                <summary>Account Address</summary>
            </details>
            <details>
                <summary>NFT Address</summary>
            </details>
        </td>
    </tr>
    <tr>
        <td>Return Amount</td>
        <td>uint64</td>
        <td>
            Amount of IOTA coins the consuming transaction should deposit to the address defined in <i>Return Address</i>.
        </td>
    </tr>
</table>

### Timelock Unlock Condition

TimelockUnlockCondition is an unlock condition which puts a time constraint on an output depending on the latest confirmed commitment's slot index T: the output can only be consumed, if T is equal or bigger than the one defined in the condition.

`Unix Time` in Timelock Unlock Condition is changed to `Slot Index`.

<table>
    <tr>
        <td><b>Name</b></td>
        <td><b>Type</b></td>
        <td><b>Description</b></td>
    </tr>
    <tr>
        <td>Unlock Condition Type</td>
        <td>uint8</td>
        <td>
            Set to <strong>value 2</strong> to denote a <i>Timelock Unlock Condition</i>.
        </td>
    </tr>
    <tr>
        <td>Slot Index</td>
        <td>uint64</td>
        <td>
            Slot index starting from which the output can be consumed.
        </td>
    </tr>
</table>

### Expiration Unlock Condition

Expiration Unlock Condition is an unlock condition which puts a time constraint on whether the receiver or return identity can consume an output depending on the latest confirmed commitment's slot index T.

<table>
    <tr>
        <td><b>Name</b></td>
        <td><b>Type</b></td>
        <td><b>Description</b></td>
    </tr>
    <tr>
        <td>Unlock Condition Type</td>
        <td>uint8</td>
        <td>
            Set to <strong>value 3</strong> to denote an <i>Expiration Unlock Condition</i>.
        </td>
    </tr>
    <tr>
        <td valign="top">Return Address <code>oneOf</code></td>
        <td colspan="2">
            <details>
                <summary>Ed25519 Address</summary>
            </details>
            <details>
                <summary>Account Address</summary>
            </details>
            <details>
                <summary>NFT Address</summary>
            </details>
        </td>
    </tr>
    <tr>
        <td>Slot Index</td>
        <td>uint64</td>
        <td>
            Before this slot index, <i>Address Unlock Condition</i> is allowed to unlock the output, after that only the address defined in <i>Return Address</i>.
        </td>
    </tr>
</table>


### State Controller Address Unlock Condition

An unlock condition defined solely for Account Output. It is functionally equivalent to an Address Unlock Condition, however there are additional transition constraints defined for the Account UTXO state machine that can only be carried out by the State Controller Address, hence it's a distinct unlock condition type.

 <table>
    <tr>
        <td><b>Name</b></td>
        <td><b>Type</b></td>
        <td><b>Description</b></td>
    </tr>
    <tr>
        <td>Unlock Condition Type</td>
        <td>uint8</td>
        <td>
            Set to <strong>value 4</strong> to denote an <i>State Controller Address Unlock Condition</i>.
        </td>
    </tr>
    <tr>
        <td>Address</td>
        <td colspan="2">
            <details>
                <summary>Ed25519 Address</summary>
            </details>
            <details>
                <summary>Account Address</summary>
            </details>
            <details>
                <summary>NFT Address</summary>
            </details>
        </td>
    </tr>
</table>

### Governor Address Unlock Condition

An unlock condition defined solely for Account Output. It is functionally equivalent to an Address Unlock Condition, however there are additional transition constraints defined for the Account UTXO state machine that can only be carried out by the Governor Address, hence it's a distinct unlock condition type.

<table>
    <tr>
        <td><b>Name</b></td>
        <td><b>Type</b></td>
        <td><b>Description</b></td>
    </tr>
    <tr>
        <td>Unlock Condition Type</td>
        <td>uint8</td>
        <td>
            Set to <strong>value 5</strong> to denote an <i>Governor Address Unlock Condition</i>.
        </td>
    </tr>
    <tr>
        <td>Address</td>
        <td colspan="2">
            <details>
                <summary>Ed25519 Address</summary>
            </details>
            <details>
                <summary>Account Address</summary>
            </details>
            <details>
                <summary>NFT Address</summary>
            </details>
        </td>
    </tr>
</table>

### Immutable Account Address Unlock Condition

An unlock condition defined for chain constrained UTXOs that can only be unlocked by a permanent Account Address.

Output unlocking is functionally equivalent to an Address Unlock Condition with an Account Address, however there are additional transition constraints: the next state of the UTXO machine must have the same Immutable Account Address Unlock Condition.

<table>
    <tr>
        <td><b>Name</b></td>
        <td><b>Type</b></td>
        <td><b>Description</b></td>
    </tr>
    <tr>
        <td>Unlock Condition Type</td>
        <td>uint8</td>
        <td>
            Set to <strong>value 6</strong> to denote an <i>Immutable Account Address Unlock Condition</i>.
        </td>
    </tr>
    <tr>
        <td>Address</td>
        <td colspan="2">
            <details>
                <summary>Account Address</summary>
                <table>
                    <tr>
                        <td><b>Name</b></td>
                        <td><b>Type</b></td>
                        <td><b>Description</b></td>
                    </tr>
                    <tr>
                        <td>Address Type</td>
                        <td>uint8</td>
                        <td>
                            Set to <strong>value 8</strong> to denote an <i>Account Address</i>.
                        </td>
                    </tr>
                    <tr>
                        <td>Account ID</td>
                        <td>ByteArray[32]</td>
                        <td>The raw bytes of the <i>Account ID</i> which is the BLAKE2b-256 hash of the outputID that created it.</td>
                    </tr>
                </table>
            </details>
        </td>
    </tr>
</table>


## Unlocks
Output and unlock designs are the same as described in [TIP-18 Multi-Asset Ledger and ISC Support](https://github.com/iotaledger/tips/blob/main/tips/TIP-0018/tip-0018.md#output-design)

No new unlock types are introduced, only on change to be made:
* Rename `alias` to `account`.

The following table lists all currently specified Unlock Condition.

<table>
    <tr>
        <th>Name</th>
        <th>Unlock Type</th>
    </tr>
    <tr>
        <td>Signature Unlock</td>
        <td>0</td>
    </tr>
    <tr>
        <td>Reference Unlock</td>
        <td>1</td>
    </tr>
    <tr>
        <td>Account Unlock</td>
        <td>2</td>
    </tr>
    <tr>
        <td>NFT Unlock</td>
        <td>3</td>
    </tr>
</table>

### Signature Unlock

A Signature Unlock defines an Unlock which holds a signature signing the BLAKE2b-256 hash of the Transaction Essence (including the optional payload).

 <table>
    <tr>
        <td><b>Name</b></td>
        <td><b>Type</b></td>
        <td><b>Description</b></td>
    </tr>
    <tr>
        <td>Unlock Type</td>
        <td>uint8</td>
        <td>
            Set to <strong>value 0</strong> to denote an <i>Signature Unlock</i>.
        </td>
    </tr>
    <tr>
        <td>Signature</td>
        <td colspan="2">
            <details>
                <summary>Ed25519 Signature</summary>
            </details>
        </td>
    </tr>
</table>

### Reference Unlock

A Reference Unlock defines an Unlock which references a previous Unlock (which must not be another Reference Unlock). It must be used if multiple inputs can be unlocked via the same Unlock.

<table>
    <tr>
        <td><b>Name</b></td>
        <td><b>Type</b></td>
        <td><b>Description</b></td>
    </tr>
    <tr>
        <td>Unlock Type</td>
        <td>uint8</td>
        <td>
            Set to <strong>value 1</strong> to denote an <i>Reference Unlock</i>.
        </td>
    </tr>
    <tr>
        <td>Reference</td>
        <td>uint16</td>
        <td>The other unlock this Reference Unlock references to.</td>
    </tr>
</table>

### Account Unlock

This unlock is similar to the Reference Unlock. However, it is valid if and only if the input of the transaction at index Account Reference Unlock Index is an alias output with the same Account ID as the one derived from the Address field of the to-be unlocked output.

 <table>
    <tr>
        <td><b>Name</b></td>
        <td><b>Type</b></td>
        <td><b>Description</b></td>
    </tr>
    <tr>
        <td>Unlock Type</td>
        <td>uint8</td>
        <td>
            Set to <strong>value 3</strong> to denote an <i>Account Unlock</i>.
        </td>
    </tr>
    <tr>
        <td>Account Reference Unlock Index</td>
        <td>uint16</td>
        <td>The other unlock this Account Unlock references to.</td>
    </tr>
</table>

### NFT Unlock

An NFT Unlock looks and behaves like an Account Unlock, but the referenced input at the index must be an NFT output with the matching NFT ID.
An NFT Unlock is only valid if the input in the transaction at index NFT Reference Unlock Index is the NFT output with the same NFT ID as the one derived from the Address field of the to-be unlocked output.

<table>
    <tr>
        <td><b>Name</b></td>
        <td><b>Type</b></td>
        <td><b>Description</b></td>
    </tr>
    <tr>
        <td>Unlock Type</td>
        <td>uint8</td>
        <td>
            Set to <strong>value 4</strong> to denote an <i>NFT Unlock</i>.
        </td>
    </tr>
    <tr>
        <td>NFT Reference Unlock Index</td>
        <td>uint16</td>
        <td>The other unlock this NFT Unlock references to.</td>
    </tr>
</table>

## Signature

The following table lists all currently supported signatures:

<table>
    <tr>
        <th>Name</th>
        <th>Signature Type</th>
    </tr>
    <tr>
        <td>Ed25519</td>
        <td>0</td>
    </tr>
</table>

### Ed25519 Signature

The following table describes the serialization of a Ed25519 signature:
<table>
    <tr>
        <th>Name</th>
        <th>Type</th>
        <th>Description</th>
    </tr>
    <tr>
        <td>Signature Type</td>
        <td>uint8</td>
        <td>The signature type of Ed25519 is 0.</td>
    </tr>
    <tr>
        <td>Public Key</td>
        <td>ByteArray[32]</td>
        <td>
            The public key used to verify the given signature.
        </td>
    </tr>
    <tr>
        <td>Signature</td>
        <td>ByteArray[64]</td>
        <td>The signature.</td>
    </tr>
</table>


