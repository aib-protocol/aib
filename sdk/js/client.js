/**
 * AIB 2.0 SDK - API Client Module
 *
 * This module provides an API client for interacting with the AIB blockchain.
 */

const http = require('http');
const https = require('https');
const { Transaction } = require('./transaction');
const { bytesToHex } = require('./wallet');

/**
 * API Client configuration options.
 */
class ClientConfig {
    constructor(options = {}) {
        this.baseURL = options.baseURL || 'http://localhost:8080/api/v1';
        this.timeout = options.timeout || 30000;
        this.apiKey = options.apiKey || null;
    }

    /**
     * Get the HTTP/HTTPS agent based on the URL.
     * @private
     */
    getAgent() {
        if (this.baseURL.startsWith('https://')) {
            return new https.Agent({
                keepAlive: true,
                timeout: this.timeout
            });
        }
        return new http.Agent({
            keepAlive: true,
            timeout: this.timeout
        });
    }
}

/**
 * API Client for interacting with AIB blockchain.
 */
class Client {
    /**
     * Create a new API client.
     * @param {Object} options - Configuration options
     */
    constructor(options = {}) {
        this.config = new ClientConfig(options);
    }

    /**
     * Make an HTTP request.
     * @private
     */
    async _request(method, path, body = null) {
        return new Promise((resolve, reject) => {
            const url = new URL(path, this.config.baseURL);
            const isHttps = url.protocol === 'https:';
            const transport = isHttps ? https : http;

            const options = {
                hostname: url.hostname,
                port: url.port || (isHttps ? 443 : 80),
                path: url.pathname + url.search,
                method: method,
                headers: {
                    'Content-Type': 'application/json',
                    'Accept': 'application/json'
                },
                timeout: this.config.timeout
            };

            if (this.config.apiKey) {
                options.headers['Authorization'] = `Bearer ${this.config.apiKey}`;
            }

            const req = transport.request(options, (res) => {
                let data = '';

                res.on('data', chunk => {
                    data += chunk;
                });

                res.on('end', () => {
                    try {
                        const json = data ? JSON.parse(data) : {};
                        if (res.statusCode >= 200 && res.statusCode < 300) {
                            resolve(json);
                        } else {
                            reject(new Error(`API error: ${res.statusCode} - ${JSON.stringify(json)}`));
                        }
                    } catch (e) {
                        reject(new Error(`Failed to parse response: ${e.message}`));
                    }
                });
            });

            req.on('error', reject);
            req.on('timeout', () => {
                req.destroy();
                reject(new Error('Request timeout'));
            });

            if (body) {
                req.write(JSON.stringify(body));
            }

            req.end();
        });
    }

    /**
     * Get balance for an address.
     * @param {string} address - Wallet address (Bech32m)
     * @returns {Promise<Object>}
     */
    async getBalance(address) {
        return await this._request('GET', `/balance/${address}`);
    }

    /**
     * Get unspent transaction outputs for an address.
     * @param {string} address - Wallet address
     * @returns {Promise<Array>}
     */
    async getUTXOs(address) {
        return await this._request('GET', `/utxos/${address}`);
    }

    /**
     * Get transaction by hash.
     * @param {string} txHash - Transaction hash (hex)
     * @returns {Promise<Object>}
     */
    async getTransaction(txHash) {
        return await this._request('GET', `/tx/${txHash}`);
    }

    /**
     * Get transaction history for an address.
     * @param {string} address - Wallet address
     * @param {number} limit - Maximum number of transactions
     * @param {number} offset - Offset for pagination
     * @returns {Promise<Array>}
     */
    async getTransactionHistory(address, limit = 50, offset = 0) {
        return await this._request('GET', `/address/${address}/txs?limit=${limit}&offset=${offset}`);
    }

    /**
     * Send a signed transaction to the network.
     * @param {Transaction} tx - Signed transaction
     * @returns {Promise<Object>}
     */
    async sendTransaction(tx) {
        const txHex = tx.serialize().toString('hex');
        return await this._request('POST', '/tx', { tx: txHex });
    }

    /**
     * Send a signed transaction from hex data.
     * @param {string} txHex - Hex-encoded transaction
     * @returns {Promise<Object>}
     */
    async sendRawTransaction(txHex) {
        return await this._request('POST', '/tx', { tx: txHex });
    }

    /**
     * Get block information.
     * @param {string} blockId - Block height or block hash
     * @returns {Promise<Object>}
     */
    async getBlock(blockId) {
        return await this._request('GET', `/block/${blockId}`);
    }

    /**
     * Get current blockchain height.
     * @returns {Promise<number>}
     */
    async getBlockHeight() {
        const info = await this.getNetworkInfo();
        return info.height;
    }

    /**
     * Get network information.
     * @returns {Promise<Object>}
     */
    async getNetworkInfo() {
        return await this._request('GET', '/network');
    }

    /**
     * Estimate transaction fee.
     * @param {number} txSize - Transaction size in bytes
     * @returns {Promise<number>}
     */
    async estimateFee(txSize) {
        return await this._request('GET', `/fee?size=${txSize}`);
    }

    /**
     * Estimate fee for a transaction.
     * @param {Transaction} tx - Transaction to estimate fee for
     * @returns {Promise<number>}
     */
    async estimateTransactionFee(tx) {
        const size = tx.serialize().length;
        return await this.estimateFee(size);
    }

    /**
     * Get mempool transactions.
     * @returns {Promise<Array>}
     */
    async getMempool() {
        return await this._request('GET', '/mempool');
    }

    /**
     * Get mempool transaction by hash.
     * @param {string} txHash - Transaction hash
     * @returns {Promise<Object>}
     */
    async getMempoolTransaction(txHash) {
        return await this._request('GET', `/mempool/${txHash}`);
    }

    /**
     * Get block transactions.
     * @param {string} blockId - Block height or hash
     * @param {number} limit - Maximum transactions
     * @param {number} offset - Offset for pagination
     * @returns {Promise<Array>}
     */
    async getBlockTransactions(blockId, limit = 50, offset = 0) {
        return await this._request('GET', `/block/${blockId}/txs?limit=${limit}&offset=${offset}`);
    }

    /**
     * Get address information including balance and tx count.
     * @param {string} address - Wallet address
     * @returns {Promise<Object>}
     */
    async getAddressInfo(address) {
        return await this._request('GET', `/address/${address}`);
    }

    /**
     * Validate an address.
     * @param {string} address - Address to validate
     * @returns {Promise<boolean>}
     */
    async validateAddress(address) {
        try {
            await this._request('GET', `/validate/${address}`);
            return true;
        } catch (e) {
            return false;
        }
    }

    /**
     * Broadcast a raw transaction hex.
     * @param {string} txHex - Hex-encoded transaction
     * @returns {Promise<string>} - Transaction hash
     */
    async broadcast(txHex) {
        const result = await this.sendRawTransaction(txHex);
        return result.tx_hash;
    }
}

/**
 * Example: Create and send a payment transaction.
 *
 * @param {Object} client - API client instance
 * @param {Object} wallet - Sender's wallet
 * @param {string} toAddress - Recipient address
 * @param {number} amount - Amount to send
 * @returns {Promise<string>} - Transaction hash
 */
async function sendPayment(client, wallet, toAddress, amount) {
    // Get UTXOs for the sender
    const utxos = await client.getUTXOs(wallet.getAddressString());

    if (utxos.length === 0) {
        throw new Error('No UTXOs available');
    }

    // Import Transaction for building
    const { Transaction, TXInput, TXOutput } = require('./transaction');

    // Build transaction inputs
    const inputs = utxos.map(utxo => new TXInput(utxo.tx_hash, utxo.index));

    // Build transaction output
    const output = new TXOutput(toAddress, amount);

    // Create and sign transaction
    const tx = new Transaction(1, inputs, [output]);
    tx.signWith(wallet);

    // Send to network
    const result = await client.sendTransaction(tx);

    return result.tx_hash;
}

/**
 * Example: Get wallet balance and transactions.
 *
 * @param {Object} client - API client instance
 * @param {string} address - Wallet address
 * @returns {Promise<Object>}
 */
async function getWalletInfo(client, address) {
    const [balance, transactions, utxos] = await Promise.all([
        client.getBalance(address),
        client.getTransactionHistory(address),
        client.getUTXOs(address)
    ]);

    return {
        balance,
        transactionCount: transactions.length,
        utxoCount: utxos.length,
        transactions: transactions.slice(0, 10) // Last 10 transactions
    };
}

// Export for Node.js
if (typeof module !== 'undefined' && module.exports) {
    module.exports = {
        Client,
        ClientConfig,
        sendPayment,
        getWalletInfo
    };
}
