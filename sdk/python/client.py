"""
AIB 2.0 SDK - API Client Module

This module provides an API client for interacting with the AIB blockchain.
"""

import json
import urllib.request
import urllib.parse
import urllib.error
from typing import List, Optional, Dict, Any
from .transaction import Transaction
from .wallet import Address


class ClientConfig:
    """API Client configuration options."""

    def __init__(
        self,
        base_url: str = "http://localhost:8080/api/v1",
        timeout: int = 30,
        api_key: Optional[str] = None
    ):
        """
        Initialize client configuration.

        Args:
            base_url: API endpoint URL
            timeout: Request timeout in seconds
            api_key: Optional API key for authentication
        """
        self.base_url = base_url.rstrip("/")
        self.timeout = timeout
        self.api_key = api_key


class Client:
    """API Client for interacting with AIB blockchain."""

    def __init__(self, config: ClientConfig = None):
        """
        Create a new API client.

        Args:
            config: Client configuration (uses defaults if not provided)
        """
        self.config = config or ClientConfig()

    def _request(self, method: str, path: str, body: Any = None) -> Dict:
        """
        Make an HTTP request.

        Args:
            method: HTTP method (GET, POST, etc.)
            path: API endpoint path
            body: Request body (will be JSON encoded)

        Returns:
            Dict: Response data

        Raises:
            Exception: If the request fails
        """
        url = f"{self.config.base_url}{path}"

        headers = {
            "Content-Type": "application/json",
            "Accept": "application/json"
        }

        if self.config.api_key:
            headers["Authorization"] = f"Bearer {self.config.api_key}"

        data = None
        if body is not None:
            data = json.dumps(body).encode("utf-8")

        request = urllib.request.Request(url, data=data, headers=headers, method=method)
        request.timeout = self.config.timeout

        try:
            with urllib.request.urlopen(request) as response:
                response_data = response.read()
                if response_data:
                    return json.loads(response_data.decode("utf-8"))
                return {}
        except urllib.error.HTTPError as e:
            error_body = e.read().decode("utf-8") if e.readable() else ""
            raise Exception(f"API error: {e.code} - {error_body}") from e
        except urllib.error.URLError as e:
            raise Exception(f"Network error: {e.reason}") from e
        except Exception as e:
            raise Exception(f"Request failed: {str(e)}") from e

    def get_balance(self, address: str) -> Dict[str, Any]:
        """
        Get balance for an address.

        Args:
            address: Wallet address (Bech32m)

        Returns:
            Dict with balance information
        """
        return self._request("GET", f"/balance/{address}")

    def get_utxos(self, address: str) -> List[Dict[str, Any]]:
        """
        Get unspent transaction outputs for an address.

        Args:
            address: Wallet address

        Returns:
            List of UTXOs
        """
        return self._request("GET", f"/utxos/{address}")

    def get_transaction(self, tx_hash: str) -> Dict[str, Any]:
        """
        Get transaction by hash.

        Args:
            tx_hash: Transaction hash (hex)

        Returns:
            Transaction details
        """
        return self._request("GET", f"/tx/{tx_hash}")

    def get_transaction_history(
        self,
        address: str,
        limit: int = 50,
        offset: int = 0
    ) -> List[Dict[str, Any]]:
        """
        Get transaction history for an address.

        Args:
            address: Wallet address
            limit: Maximum number of transactions
            offset: Offset for pagination

        Returns:
            List of transactions
        """
        params = urllib.parse.urlencode({"limit": limit, "offset": offset})
        return self._request("GET", f"/address/{address}/txs?{params}")

    def send_transaction(self, tx: Transaction) -> Dict[str, Any]:
        """
        Send a signed transaction to the network.

        Args:
            tx: Signed transaction

        Returns:
            Response with transaction hash
        """
        tx_hex = tx.serialize().hex()
        return self._request("POST", "/tx", {"tx": tx_hex})

    def send_raw_transaction(self, tx_hex: str) -> Dict[str, Any]:
        """
        Send a signed transaction from hex data.

        Args:
            tx_hex: Hex-encoded transaction

        Returns:
            Response with transaction hash
        """
        return self._request("POST", "/tx", {"tx": tx_hex})

    def get_block(self, block_id: str) -> Dict[str, Any]:
        """
        Get block information.

        Args:
            block_id: Block height or block hash

        Returns:
            Block details
        """
        return self._request("GET", f"/block/{block_id}")

    def get_block_height(self) -> int:
        """
        Get current blockchain height.

        Returns:
            Block height
        """
        info = self.get_network_info()
        return info.get("height", 0)

    def get_network_info(self) -> Dict[str, Any]:
        """
        Get network information.

        Returns:
            Network status
        """
        return self._request("GET", "/network")

    def estimate_fee(self, tx_size: int) -> int:
        """
        Estimate transaction fee.

        Args:
            tx_size: Transaction size in bytes

        Returns:
            Estimated fee in smallest units
        """
        result = self._request("GET", f"/fee?size={tx_size}")
        return result.get("fee", 0)

    def estimate_transaction_fee(self, tx: Transaction) -> int:
        """
        Estimate fee for a transaction.

        Args:
            tx: Transaction to estimate fee for

        Returns:
            Estimated fee
        """
        size = len(tx.serialize())
        return self.estimate_fee(size)

    def get_mempool(self) -> List[Dict[str, Any]]:
        """
        Get mempool transactions.

        Returns:
            List of mempool transactions
        """
        return self._request("GET", "/mempool")

    def get_mempool_transaction(self, tx_hash: str) -> Dict[str, Any]:
        """
        Get mempool transaction by hash.

        Args:
            tx_hash: Transaction hash

        Returns:
            Mempool transaction details
        """
        return self._request("GET", f"/mempool/{tx_hash}")

    def get_block_transactions(
        self,
        block_id: str,
        limit: int = 50,
        offset: int = 0
    ) -> List[Dict[str, Any]]:
        """
        Get block transactions.

        Args:
            block_id: Block height or hash
            limit: Maximum transactions
            offset: Offset for pagination

        Returns:
            List of transactions
        """
        params = urllib.parse.urlencode({"limit": limit, "offset": offset})
        return self._request("GET", f"/block/{block_id}/txs?{params}")

    def get_address_info(self, address: str) -> Dict[str, Any]:
        """
        Get address information including balance and tx count.

        Args:
            address: Wallet address

        Returns:
            Address information
        """
        return self._request("GET", f"/address/{address}")

    def validate_address(self, address: str) -> bool:
        """
        Validate an address.

        Args:
            address: Address to validate

        Returns:
            True if valid, False otherwise
        """
        try:
            self._request("GET", f"/validate/{address}")
            return True
        except Exception:
            return False

    def broadcast(self, tx_hex: str) -> str:
        """
        Broadcast a raw transaction hex.

        Args:
            tx_hex: Hex-encoded transaction

        Returns:
            Transaction hash
        """
        result = self.send_raw_transaction(tx_hex)
        return result.get("tx_hash", "")


def send_payment(
    client: Client,
    wallet,
    to_address: str,
    amount: int
) -> str:
    """
    Create and send a payment transaction.

    Args:
        client: API client instance
        wallet: Sender's wallet
        to_address: Recipient address
        amount: Amount to send

    Returns:
        Transaction hash

    Raises:
        Exception: If no UTXOs available or transaction fails
    """
    # Get UTXOs for the sender
    utxos = client.get_utxos(wallet.get_address_string())

    if not utxos:
        raise Exception("No UTXOs available")

    # Import Transaction for building
    from .transaction import Transaction, TXInput, TXOutput

    # Build transaction inputs
    inputs = [TXInput(utxo["tx_hash"], utxo["index"]) for utxo in utxos]

    # Build transaction output
    output = TXOutput(to_address, amount)

    # Create and sign transaction
    tx = Transaction(1, inputs, [output])
    tx.sign_with(wallet)

    # Send to network
    result = client.send_transaction(tx)

    return result.get("tx_hash", "")


def get_wallet_info(client: Client, address: str) -> Dict[str, Any]:
    """
    Get wallet balance and transactions.

    Args:
        client: API client instance
        address: Wallet address

    Returns:
        Wallet information including balance and recent transactions
    """
    balance = client.get_balance(address)
    transactions = client.get_transaction_history(address)
    utxos = client.get_utxos(address)

    return {
        "balance": balance,
        "transaction_count": len(transactions),
        "utxo_count": len(utxos),
        "recent_transactions": transactions[:10]  # Last 10 transactions
    }


# Example usage
if __name__ == "__main__":
    # Create client
    client = Client(ClientConfig(base_url="http://localhost:8080/api/v1"))

    # Get network info
    info = client.get_network_info()
    print(f"Network: {info}")

    # Get balance (example address)
    # balance = client.get_balance("aib1...")
    # print(f"Balance: {balance}")
