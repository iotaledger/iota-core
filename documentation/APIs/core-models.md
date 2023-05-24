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
  * [Transaction](#transaction)
  * [Outputs and Unlocks](#outputs-and-unlocks)
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
            <td>The time at which this transaction was created by the client. It's a Unix-like timestamp in nanosecond.</td>
          </tr>
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
                <summary>Alias Output</summary>
                <blockquote>
                  Describes an alias account in the ledger.
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
        <summary>Alias Unlock</summary>
        <blockquote>
          References a previous unlock of a consumed alias output.
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

Transaction validation rules follow [TIP 20 Transaction Payload with TIP-18 Output Types](https://github.com/iotaledger/tips/blob/ae7fbd336e506f907080aa6b16d6ffb0d7a5553c/tips/TIP-0020/tip-0020.md).

### Outputs and Unlocks
Output and unlock designs are the same as described in [TIP-18 Multi-Asset Ledger and ISC Support](https://github.com/iotaledger/tips/blob/main/tips/TIP-0018/tip-0018.md#output-design)

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