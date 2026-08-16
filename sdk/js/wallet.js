/**
 * AIB 2.0 SDK for JavaScript/TypeScript
 *
 * AIB 2.0 Specifications:
 *   - Cryptographic Algorithm: Ed25519
 *   - Address Format: Bech32m (HRP: "aib")
 *   - Transaction Model: UTXO
 *
 * @module aib-sdk
 */

// Import crypto libraries - using Web Crypto API for browser and Node.js
const crypto = globalThis.crypto || require('crypto').webcrypto;

/**
 * Address represents a 32-byte AIB blockchain address.
 */
class Address {
    constructor(bytes) {
        if (bytes.length !== 32) {
            throw new Error('Address must be 32 bytes');
        }
        this.bytes = bytes;
    }

    /**
     * Convert address to hex string.
     */
    toHex() {
        return Buffer.from(this.bytes).toString('hex');
    }

    /**
     * Convert address to Bech32m format.
     */
    toBech32() {
        return encodeBech32m('aib', this.bytes);
    }

    /**
     * Create address from Bech32m string.
     */
    static fromBech32(bech32String) {
        const bytes = decodeBech32m(bech32String);
        return new Address(bytes);
    }
}

/**
 * Wallet represents a cryptographic wallet with Ed25519 key pair.
 */
class Wallet {
    /**
     * Create a new wallet with a randomly generated key pair.
     * @returns {Promise<Wallet>}
     */
    static async create() {
        const keyPair = await crypto.subtle.generateKey(
            {
                name: 'Ed25519',
                publicKeyUsage: [],
                privateKeyUsage: ['sign']
            },
            true,
            ['sign', 'verify']
        );

        const publicKeyBuffer = await crypto.subtle.exportKey('raw', keyPair.publicKey);

        return new Wallet(keyPair.privateKey, keyPair.publicKey, publicKeyBuffer);
    }

    /**
     * Create a wallet from a 32-byte seed.
     * @param {Uint8Array} seed - 32-byte seed
     * @returns {Promise<Wallet>}
     */
    static async fromSeed(seed) {
        if (seed.length !== 32) {
            throw new Error('Seed must be 32 bytes');
        }

        // Derive key from seed using HKDF-like derivation
        const keyMaterial = await crypto.subtle.importKey(
            'raw',
            seed,
            'HKDF',
            false,
            ['deriveKey']
        );

        const keyPair = await crypto.subtle.deriveKey(
            {
                name: 'Ed25519',
                salt: new Uint8Array(0),
                info: new TextEncoder().encode('AIB-Wallet'),
                hash: 'SHA-256'
            },
            keyMaterial,
            {
                name: 'Ed25519',
                publicKeyUsage: [],
                privateKeyUsage: ['sign']
            },
            true,
            ['sign', 'verify']
        );

        const publicKeyBuffer = await crypto.subtle.exportKey('raw', keyPair.publicKey);

        return new Wallet(keyPair.privateKey, keyPair.publicKey, publicKeyBuffer);
    }

    /**
     * Import a wallet from an existing private key.
     * @param {string} privateKeyHex - Private key as hex string
     * @returns {Promise<Wallet>}
     */
    static async fromPrivateKey(privateKeyHex) {
        const privateKeyBytes = hexToBytes(privateKeyHex);

        if (privateKeyBytes.length !== 32) {
            throw new Error('Private key must be 32 bytes');
        }

        const keyPair = await crypto.subtle.importKey(
            'raw',
            privateKeyBytes,
            {
                name: 'Ed25519',
                publicKeyUsage: [],
                privateKeyUsage: ['sign']
            },
            true,
            ['sign', 'verify']
        );

        const publicKeyBuffer = await crypto.subtle.exportKey('raw', keyPair.publicKey);

        return new Wallet(keyPair, keyPair.public(), publicKeyBuffer);
    }

    /**
     * @param {CryptoKey} privateKey
     * @param {CryptoKey} publicKey
     * @param {ArrayBuffer} publicKeyBytes
     */
    constructor(privateKey, publicKey, publicKeyBytes) {
        this.privateKey = privateKey;
        this.publicKey = publicKey;
        this.publicKeyBytes = new Uint8Array(publicKeyBytes);

        // Derive address from public key using SHA256
        this.address = this._deriveAddress();
    }

    /**
     * Derive address from public key using SHA256.
     * @private
     */
    async _deriveAddress() {
        const hashBuffer = await crypto.subtle.digest('SHA-256', this.publicKeyBytes);
        return new Address(new Uint8Array(hashBuffer));
    }

    /**
     * Get the wallet's address.
     * @returns {Address}
     */
    getAddress() {
        return this.address;
    }

    /**
     * Get the wallet's address as a Bech32m string.
     * @returns {string}
     */
    getAddressString() {
        return this.address.toBech32();
    }

    /**
     * Get the wallet's public key.
     * @returns {Uint8Array}
     */
    getPublicKey() {
        return this.publicKeyBytes;
    }

    /**
     * Get the wallet's private key (WARNING: Keep this secure!).
     * @returns {Promise<Uint8Array>}
     */
    async getPrivateKey() {
        const exported = await crypto.subtle.exportKey('raw', this.privateKey);
        return new Uint8Array(exported);
    }

    /**
     * Get the wallet's private key as hex string (WARNING: Keep this secure!).
     * @returns {Promise<string>}
     */
    async getPrivateKeyHex() {
        const privateKey = await this.getPrivateKey();
        return bytesToHex(privateKey);
    }

    /**
     * Sign a message with the wallet's private key.
     * @param {Uint8Array} message - Message to sign
     * @returns {Promise<Uint8Array>}
     */
    async sign(message) {
        const signature = await crypto.subtle.sign(
            {
                name: 'Ed25519'
            },
            this.privateKey,
            message
        );
        return new Uint8Array(signature);
    }

    /**
     * Verify a signature for a message.
     * @param {Uint8Array} message - Original message
     * @param {Uint8Array} signature - Signature to verify
     * @returns {Promise<boolean>}
     */
    async verify(message, signature) {
        return await crypto.subtle.verify(
            {
                name: 'Ed25519'
            },
            this.publicKey,
            signature,
            message
        );
    }

    /**
     * Sign a transaction with the wallet's private key.
     * @param {Transaction} tx - Transaction to sign
     */
    async signTransaction(tx) {
        const txHash = tx.hash();
        const signature = await this.sign(new Uint8Array(txHash));

        // Add signature and public key to all inputs
        for (const input of tx.inputs) {
            input.signature = signature;
            input.publicKey = this.publicKeyBytes;
        }
    }
}

/**
 * UTXO input for transactions.
 */
class TXInput {
    constructor(txHash, index) {
        this.txHash = txHash; // 32-byte transaction hash
        this.index = index;   // Output index
        this.signature = null;
        this.publicKey = null;
        this.sequence = 0xFFFFFFFFFFFFFFFFn;
    }
}

/**
 * UTXO output for transactions.
 */
class TXOutput {
    constructor(address, amount, assetId = null) {
        this.address = address; // Address instance or string
        this.amount = amount;   // Amount in smallest units
        this.assetId = assetId; // Optional asset ID (32 bytes)
        this.metadata = null;
    }
}

/**
 * Transaction represents a UTXO-based transaction.
 */
class Transaction {
    /**
     * Create a new transaction.
     * @param {number} version - Transaction version
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
     * @param {Object[]} inputs - Input parameters
     * @param {Object[]} outputs - Output parameters
     * @returns {Transaction}
     */
    static build(inputs, outputs) {
        const txInputs = inputs.map(inp => {
            const txHashBytes = hexToBytes(inp.txHash);
            return new TXInput(txHashBytes, inp.index);
        });

        const txOutputs = outputs.map(out => {
            let address;
            if (typeof out.address === 'string') {
                address = Address.fromBech32(out.address);
            } else {
                address = out.address;
            }
            return new TXOutput(address, out.amount, out.assetId);
        });

        return new Transaction(1, txInputs, txOutputs);
    }

    /**
     * Compute the transaction hash (transaction ID).
     * @returns {Promise<Uint8Array>}
     */
    async hash() {
        const serialized = await this.serialize();
        const hash1 = await crypto.subtle.digest('SHA-256', serialized);
        const hash2 = await crypto.subtle.digest('SHA-256', hash1);
        return new Uint8Array(hash2);
    }

    /**
     * Serialize the transaction to bytes.
     * @returns {Promise<Uint8Array>}
     */
    async serialize() {
        const buffers = [];

        // Version (4 bytes, little-endian)
        const versionBuf = new ArrayBuffer(4);
        const versionView = new DataView(versionBuf);
        versionView.setUint32(0, this.version, true);
        buffers.push(versionBuf);

        // Number of inputs (8 bytes, varint-like)
        buffers.push(varintEncode(this.inputs.length));

        // Inputs
        for (const input of this.inputs) {
            // Previous tx hash (32 bytes)
            buffers.push(input.txHash);

            // Index (4 bytes)
            const indexBuf = new ArrayBuffer(4);
            const indexView = new DataView(indexBuf);
            indexView.setUint32(0, input.index, true);
            buffers.push(indexBuf);

            // Signature
            buffers.push(varintEncode(input.signature ? input.signature.length : 0));
            if (input.signature) {
                buffers.push(input.signature);
            }

            // Public key
            buffers.push(varintEncode(input.publicKey ? input.publicKey.length : 0));
            if (input.publicKey) {
                buffers.push(input.publicKey);
            }

            // Sequence (8 bytes)
            const seqBuf = new ArrayBuffer(8);
            const seqView = new DataView(seqBuf);
            seqView.setBigUint64(0, input.sequence, true);
            buffers.push(seqBuf);
        }

        // Number of outputs
        buffers.push(varintEncode(this.outputs.length));

        // Outputs
        for (const output of this.outputs) {
            const addr = typeof output.address === 'string'
                ? output.address.bytes
                : output.address.bytes;
            buffers.push(addr);

            // Amount (8 bytes)
            const amountBuf = new ArrayBuffer(8);
            const amountView = new DataView(amountBuf);
            amountView.setUint64(0, output.amount, true);
            buffers.push(amountBuf);

            // Asset ID (32 bytes)
            const assetIdBuf = output.assetId || new ArrayBuffer(32);
            buffers.push(assetIdBuf);

            // Metadata
            buffers.push(varintEncode(output.metadata ? output.metadata.length : 0));
            if (output.metadata) {
                buffers.push(output.metadata);
            }
        }

        // LockTime (4 bytes)
        const lockBuf = new ArrayBuffer(4);
        const lockView = new DataView(lockBuf);
        lockView.setUint32(0, this.lockTime, true);
        buffers.push(lockBuf);

        // Concatenate all buffers
        const totalLength = buffers.reduce((sum, buf) => sum + buf.byteLength, 0);
        const result = new Uint8Array(totalLength);
        let offset = 0;
        for (const buf of buffers) {
            result.set(new Uint8Array(buf), offset);
            offset += buf.byteLength;
        }

        return result;
    }

    /**
     * Get transaction ID as hex string.
     * @returns {Promise<string>}
     */
    async getTxId() {
        const hash = await this.hash();
        return bytesToHex(hash);
    }
}

// ============= Helper Functions =============

/**
 * Convert hex string to bytes.
 */
function hexToBytes(hex) {
    const bytes = new Uint8Array(hex.length / 2);
    for (let i = 0; i < hex.length; i += 2) {
        bytes[i / 2] = parseInt(hex.substr(i, 2), 16);
    }
    return bytes;
}

/**
 * Convert bytes to hex string.
 */
function bytesToHex(bytes) {
    return Buffer.from(bytes).toString('hex');
}

/**
 * Encode number as varint-like encoding.
 */
function varintEncode(num) {
    const buf = [];
    while (num > 0x7F) {
        buf.push((num & 0x7F) | 0x80);
        num = Math.floor(num / 128);
    }
    buf.push(num);
    return new Uint8Array(buf);
}

/**
 * Simplified Bech32m encoding.
 * For production, use a proper bech32m library.
 */
function encodeBech32m(hrp, data) {
    const charset = 'qpzry9x8gf2tvdw0s3jn54khce6mua7l';

    // Compute checksum
    const checksumData = new Uint8Array([...textToBytes(hrp), ...data]);
    const checksum = sha256(sha256(checksumData));

    // Take 5-bit chunks
    let encoded = '';
    for (let i = 0; i < data.length * 8 - 3; i += 5) {
        const byteIndex = Math.floor(i / 8);
        const bitOffset = 8 - (i % 8) - 5;
        let index = (data[byteIndex] >> (bitOffset > 0 ? bitOffset : 0)) & 0x1F;
        if (bitOffset < 0) {
            index = (data[byteIndex] << (-bitOffset)) | (data[byteIndex + 1] >> (8 + bitOffset));
            index &= 0x1F;
        }
        encoded += charset[index];
    }

    // Add checksum (6 characters)
    for (let i = 0; i < 6; i++) {
        encoded += charset[checksum[i] % 32];
    }

    return hrp + '1' + encoded;
}

/**
 * Simplified Bech32m decoding.
 */
function decodeBech32m(address) {
    if (address.length < 6) {
        throw new Error('Address too short');
    }

    const separatorIndex = address.indexOf('1');
    if (separatorIndex === -1) {
        throw new Error('Invalid address format');
    }

    const hrp = address.slice(0, separatorIndex);
    if (hrp !== 'aib') {
        throw new Error(`Invalid HRP: expected 'aib', got '${hrp}'`);
    }

    const charset = 'qpzry9x8gf2tvdw0s3jn54khce6mua7l';
    const dataStr = address.slice(separatorIndex + 1, -6); // Remove checksum

    const data = [];
    let buffer = 0;
    let bitsLeft = 0;

    for (const c of dataStr) {
        const index = charset.indexOf(c);
        if (index === -1) {
            throw new Error(`Invalid character: ${c}`);
        }

        buffer = (buffer << 5) | index;
        bitsLeft += 5;

        if (bitsLeft >= 8) {
            bitsLeft -= 8;
            data.push((buffer >> bitsLeft) & 0xFF);
        }
    }

    return new Uint8Array(data.slice(0, 32));
}

/**
 * SHA256 hash (synchronous for Node.js compatibility).
 */
function sha256(data) {
    const crypto = require('crypto');
    const hash = crypto.createHash('sha256');
    hash.update(Buffer.from(data));
    return new Uint8Array(hash.digest());
}

/**
 * Convert text to bytes.
 */
function textToBytes(text) {
    return new TextEncoder().encode(text);
}

// Export for Node.js
if (typeof module !== 'undefined' && module.exports) {
    module.exports = { Wallet, Address, Transaction, TXInput, TXOutput };
}
