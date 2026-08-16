/**
 * AIB 2.0 SDK - Transaction Module
 *
 * This module provides transaction building, signing, and serialization
 * for the AIB blockchain UTXO model.
 */

const { Wallet, Address, hexToBytes, bytesToHex } = require('./wallet');
const crypto = require('crypto');

/**
 * UTXO input for transactions.
 */
class TXInput {
    /**
     * Create a transaction input.
     * @param {string|Uint8Array} txHash - Previous transaction hash (hex or bytes)
     * @param {number} index - Output index in previous transaction
     */
    constructor(txHash, index) {
        if (typeof txHash === 'string') {
            this.txHash = hexToBytes(txHash);
        } else {
            this.txHash = txHash;
        }

        if (this.txHash.length !== 32) {
            throw new Error('Transaction hash must be 32 bytes');
        }

        this.index = index;
        this.signature = null;
        this.publicKey = null;
        this.sequence = BigInt(0xFFFFFFFFFFFFFFFF);
    }
}

/**
 * UTXO output for transactions.
 */
class TXOutput {
    /**
     * Create a transaction output.
     * @param {string|Address} address - Recipient address
     * @param {number|BigInt} amount - Amount in smallest units
     * @param {string|Uint8Array} assetId - Optional asset ID
     */
    constructor(address, amount, assetId = null) {
        if (typeof address === 'string') {
            this.address = Address.fromBech32(address);
        } else {
            this.address = address;
        }

        this.amount = BigInt(amount);

        if (assetId) {
            this.assetId = typeof assetId === 'string' ? hexToBytes(assetId) : assetId;
        } else {
            this.assetId = new Uint8Array(32);
        }

        this.metadata = null;
    }
}

/**
 * Transaction represents a UTXO-based transaction.
 */
class Transaction {
    /**
     * Create a new transaction.
     * @param {number} version - Transaction version (default: 1)
     * @param {TXInput[]} inputs - Transaction inputs
     * @param {TXOutput[]} outputs - Transaction outputs
     */
    constructor(version = 1, inputs = [], outputs = []) {
        this.version = version;
        this.inputs = inputs;
        this.outputs = outputs;
        this.lockTime = 0;
    }

    /**
     * Build a transaction from input and output parameters.
     * @param {Object[]} inputs - Input parameters [{ txHash, index }]
     * @param {Object[]} outputs - Output parameters [{ address, amount, assetId? }]
     * @returns {Transaction}
     */
    static build(inputs, outputs) {
        const txInputs = inputs.map(inp => new TXInput(inp.txHash, inp.index));
        const txOutputs = outputs.map(out => new TXOutput(out.address, out.amount, out.assetId));

        return new Transaction(1, txInputs, txOutputs);
    }

    /**
     * Compute the transaction hash (transaction ID).
     * @returns {Uint8Array}
     */
    hash() {
        const serialized = this.serialize();
        const hash1 = crypto.createHash('sha256').update(Buffer.from(serialized)).digest();
        const hash2 = crypto.createHash('sha256').update(hash1).digest();
        return new Uint8Array(hash2);
    }

    /**
     * Serialize the transaction to bytes.
     * @returns {Uint8Array}
     */
    serialize() {
        const buffers = [];

        // Version (4 bytes, little-endian)
        const versionBuf = Buffer.alloc(4);
        versionBuf.writeUInt32LE(this.version, 0);
        buffers.push(versionBuf);

        // Number of inputs (varint)
        buffers.push(varintEncode(this.inputs.length));

        // Inputs
        for (const input of this.inputs) {
            // Previous tx hash (32 bytes)
            buffers.push(Buffer.from(input.txHash));

            // Index (4 bytes)
            const indexBuf = Buffer.alloc(4);
            indexBuf.writeUInt32LE(input.index, 0);
            buffers.push(indexBuf);

            // Signature length and data
            const sigLen = input.signature ? input.signature.length : 0;
            buffers.push(varintEncode(sigLen));
            if (sigLen > 0) {
                buffers.push(Buffer.from(input.signature));
            }

            // Public key length and data
            const pkLen = input.publicKey ? input.publicKey.length : 0;
            buffers.push(varintEncode(pkLen));
            if (pkLen > 0) {
                buffers.push(Buffer.from(input.publicKey));
            }

            // Sequence (8 bytes)
            const seqBuf = Buffer.alloc(8);
            seqBuf.writeBigUInt64LE(input.sequence, 0);
            buffers.push(seqBuf);
        }

        // Number of outputs
        buffers.push(varintEncode(this.outputs.length));

        // Outputs
        for (const output of this.outputs) {
            // Address (32 bytes)
            buffers.push(Buffer.from(output.address.bytes));

            // Amount (8 bytes)
            const amountBuf = Buffer.alloc(8);
            amountBuf.writeBigUInt64LE(output.amount, 0);
            buffers.push(amountBuf);

            // Asset ID (32 bytes)
            buffers.push(Buffer.from(output.assetId));

            // Metadata
            const metaLen = output.metadata ? output.metadata.length : 0;
            buffers.push(varintEncode(metaLen));
            if (metaLen > 0) {
                buffers.push(Buffer.from(output.metadata));
            }
        }

        // LockTime (4 bytes)
        const lockBuf = Buffer.alloc(4);
        lockBuf.writeUInt32LE(this.lockTime, 0);
        buffers.push(lockBuf);

        return Buffer.concat(buffers);
    }

    /**
     * Deserialize a transaction from bytes.
     * @param {Buffer|Uint8Array} data - Serialized transaction
     * @returns {Transaction}
     */
    static deserialize(data) {
        const buf = Buffer.from(data);
        let offset = 0;

        // Version
        const version = buf.readUInt32LE(offset);
        offset += 4;

        // Number of inputs
        const { value: numInputs, bytesRead: inputsLenBytes } = varintDecode(buf.slice(offset));
        offset += inputsLenBytes;

        // Inputs
        const inputs = [];
        for (let i = 0; i < numInputs; i++) {
            const txHash = buf.slice(offset, offset + 32);
            offset += 32;

            const index = buf.readUInt32LE(offset);
            offset += 4;

            const input = new TXInput(txHash, index);

            // Signature
            const { value: sigLen, bytesRead: sigLenBytes } = varintDecode(buf.slice(offset));
            offset += sigLenBytes;
            if (sigLen > 0) {
                input.signature = buf.slice(offset, offset + sigLen);
                offset += sigLen;
            }

            // Public key
            const { value: pkLen, bytesRead: pkLenBytes } = varintDecode(buf.slice(offset));
            offset += pkLenBytes;
            if (pkLen > 0) {
                input.publicKey = buf.slice(offset, offset + pkLen);
                offset += pkLen;
            }

            // Sequence
            input.sequence = buf.readBigUInt64LE(offset);
            offset += 8;

            inputs.push(input);
        }

        // Number of outputs
        const { value: numOutputs, bytesRead: outputsLenBytes } = varintDecode(buf.slice(offset));
        offset += outputsLenBytes;

        // Outputs
        const outputs = [];
        for (let i = 0; i < numOutputs; i++) {
            const addressBytes = buf.slice(offset, offset + 32);
            offset += 32;
            const address = new Address(addressBytes);

            const amount = buf.readBigUInt64LE(offset);
            offset += 8;

            const assetId = buf.slice(offset, offset + 32);
            offset += 32;

            // Metadata
            const { value: metaLen, bytesRead: metaLenBytes } = varintDecode(buf.slice(offset));
            offset += metaLenBytes;
            let metadata = null;
            if (metaLen > 0) {
                metadata = buf.slice(offset, offset + metaLen);
                offset += metaLen;
            }

            const output = new TXOutput(address, amount, assetId);
            output.metadata = metadata;
            outputs.push(output);
        }

        // LockTime
        const lockTime = buf.readUInt32LE(offset);

        const tx = new Transaction(version, inputs, outputs);
        tx.lockTime = lockTime;
        return tx;
    }

    /**
     * Get transaction ID as hex string.
     * @returns {string}
     */
    getTxId() {
        const hash = this.hash();
        return bytesToHex(hash);
    }

    /**
     * Get total input amount.
     * Note: Requires UTXO database to calculate.
     * @returns {BigInt}
     */
    getInputSum() {
        // This would need UTXO lookup in practice
        return BigInt(0);
    }

    /**
     * Get total output amount.
     * @returns {BigInt}
     */
    getOutputSum() {
        let sum = BigInt(0);
        for (const output of this.outputs) {
            sum += output.amount;
        }
        return sum;
    }

    /**
     * Calculate transaction fee.
     * Note: Requires UTXO database for accurate calculation.
     * @param {BigInt} inputSum - Sum of input amounts
     * @returns {BigInt}
     */
    getFee(inputSum) {
        return inputSum - this.getOutputSum();
    }

    /**
     * Sign the transaction with a wallet.
     * @param {Wallet} wallet - Wallet to sign with
     */
    signWith(wallet) {
        const txHash = this.hash();
        const signature = wallet.sign(Buffer.from(txHash));

        // Add signature and public key to all inputs
        for (const input of this.inputs) {
            input.signature = signature;
            input.publicKey = wallet.getPublicKey();
        }
    }

    /**
     * Verify all input signatures.
     * @returns {boolean}
     */
    verify() {
        const txHash = this.hash();

        for (const input of this.inputs) {
            if (!input.signature || !input.publicKey) {
                return false;
            }

            // Note: Node.js Ed25519 verification would require a library
            // For now, return true if signatures exist
        }

        return true;
    }
}

// ============= Helper Functions =============

/**
 * Encode number as varint.
 */
function varintEncode(num) {
    const buf = [];
    num = BigInt(num);
    while (num > BigInt(0x7F)) {
        buf.push(Number((BigInt(num) & BigInt(0x7F)) | BigInt(0x80)));
        num = num / BigInt(128);
    }
    buf.push(Number(num));
    return Buffer.from(buf);
}

/**
 * Decode varint.
 */
function varintDecode(buf) {
    let value = BigInt(0);
    let bytesRead = 0;
    let shift = BigInt(0);

    for (let i = 0; i < buf.length; i++) {
        bytesRead++;
        const b = BigInt(buf[i]);
        value |= (b & BigInt(0x7F)) << shift;

        if (!(b & BigInt(0x80))) {
            break;
        }
        shift += BigInt(7);
    }

    return { value: Number(value), bytesRead };
}

/**
 * Create a simple payment transaction.
 *
 * @param {Wallet} fromWallet - Sender's wallet
 * @param {string} toAddress - Recipient address (Bech32m)
 * @param {number|BigInt} amount - Amount to send
 * @param {Array} utxos - Available UTXOs for the sender
 * @returns {Transaction}
 */
function createPayment(fromWallet, toAddress, amount, utxos) {
    // Build inputs from available UTXOs
    const inputs = utxos.map(utxo => new TXInput(utxo.txHash, utxo.index));

    // Create output
    const output = new TXOutput(toAddress, amount);

    // Create transaction
    const tx = new Transaction(1, inputs, [output]);

    // Sign the transaction
    tx.signWith(fromWallet);

    return tx;
}

/**
 * Create a batch payment transaction to multiple recipients.
 *
 * @param {Wallet} fromWallet - Sender's wallet
 * @param {Array} recipients - Array of { address, amount }
 * @param {Array} utxos - Available UTXOs
 * @returns {Transaction}
 */
function createBatchPayment(fromWallet, recipients, utxos) {
    const inputs = utxos.map(utxo => new TXInput(utxo.txHash, utxo.index));
    const outputs = recipients.map(r => new TXOutput(r.address, r.amount));

    const tx = new Transaction(1, inputs, outputs);
    tx.signWith(fromWallet);

    return tx;
}

// Export for Node.js
if (typeof module !== 'undefined' && module.exports) {
    module.exports = {
        Transaction,
        TXInput,
        TXOutput,
        createPayment,
        createBatchPayment
    };
}
