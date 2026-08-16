"""
AIB 2.0 SDK - Transaction Module

This module provides transaction building, signing, and serialization
for the AIB blockchain UTXO model.
"""

import struct
import hashlib
from typing import List, Optional, Union
from .wallet import Wallet, Address


class TXInput:
    """
    UTXO input for transactions.
    Represents a reference to a previous transaction output.
    """

    def __init__(self, tx_hash: Union[str, bytes], index: int):
        """
        Create a transaction input.

        Args:
            tx_hash: Previous transaction hash (hex string or bytes)
            index: Output index in previous transaction
        """
        if isinstance(tx_hash, str):
            self.tx_hash = bytes.fromhex(tx_hash)
        else:
            self.tx_hash = tx_hash

        if len(self.tx_hash) != 32:
            raise ValueError("Transaction hash must be 32 bytes")

        self.index = index
        self.signature: Optional[bytes] = None
        self.public_key: Optional[bytes] = None
        self.sequence = 0xFFFFFFFFFFFFFFFF

    def __repr__(self) -> str:
        return f"TXInput(tx_hash={self.tx_hash.hex()[:16]}..., index={self.index})"


class TXOutput:
    """
    UTXO output for transactions.
    Represents a new unspent transaction output.
    """

    def __init__(
        self,
        address: Union[str, Address],
        amount: int,
        asset_id: Optional[Union[str, bytes]] = None
    ):
        """
        Create a transaction output.

        Args:
            address: Recipient address (Bech32m string or Address)
            amount: Amount in smallest units
            asset_id: Optional asset ID (32 bytes)
        """
        if isinstance(address, str):
            self.address = Address.from_bech32(address)
        else:
            self.address = address

        self.amount = amount

        if asset_id:
            self.asset_id = bytes.fromhex(asset_id) if isinstance(asset_id, str) else asset_id
        else:
            self.asset_id = bytes(32)

        self.metadata: Optional[bytes] = None

    def __repr__(self) -> str:
        return f"TXOutput(address={self.address.to_bech32()[:20]}..., amount={self.amount})"


class Transaction:
    """
    Represents a UTXO-based transaction.
    """

    def __init__(self, version: int = 1, inputs: List[TXInput] = None, outputs: List[TXOutput] = None):
        """
        Create a new transaction.

        Args:
            version: Transaction version (default: 1)
            inputs: List of transaction inputs
            outputs: List of transaction outputs
        """
        self.version = version
        self.inputs = inputs or []
        self.outputs = outputs or []
        self.lock_time = 0

    @staticmethod
    def build(inputs: List[dict], outputs: List[dict]) -> "Transaction":
        """
        Build a transaction from input and output parameters.

        Args:
            inputs: List of input dicts with keys: tx_hash, index
            outputs: List of output dicts with keys: address, amount, asset_id?

        Returns:
            Transaction: New transaction instance
        """
        tx_inputs = [TXInput(inp["tx_hash"], inp["index"]) for inp in inputs]
        tx_outputs = [
            TXOutput(out["address"], out["amount"], out.get("asset_id"))
            for out in outputs
        ]

        return Transaction(1, tx_inputs, tx_outputs)

    def hash(self) -> bytes:
        """
        Compute the transaction hash (transaction ID).
        Uses double SHA256.

        Returns:
            bytes: 32-byte transaction hash
        """
        serialized = self.serialize()
        return hashlib.sha256(hashlib.sha256(serialized).digest()).digest()

    def serialize(self) -> bytes:
        """
        Serialize the transaction to bytes.

        Returns:
            bytes: Serialized transaction
        """
        data = bytearray()

        # Version (4 bytes, little-endian)
        data.extend(struct.pack("<I", self.version))

        # Number of inputs (varint)
        data.extend(self._varint_encode(len(self.inputs)))

        # Inputs
        for inp in self.inputs:
            # Previous tx hash (32 bytes)
            data.extend(inp.tx_hash)

            # Index (4 bytes)
            data.extend(struct.pack("<I", inp.index))

            # Signature
            sig_len = len(inp.signature) if inp.signature else 0
            data.extend(self._varint_encode(sig_len))
            if sig_len > 0:
                data.extend(inp.signature)

            # Public key
            pk_len = len(inp.public_key) if inp.public_key else 0
            data.extend(self._varint_encode(pk_len))
            if pk_len > 0:
                data.extend(inp.public_key)

            # Sequence (8 bytes)
            data.extend(struct.pack("<Q", inp.sequence))

        # Number of outputs
        data.extend(self._varint_encode(len(self.outputs)))

        # Outputs
        for out in self.outputs:
            # Address (32 bytes)
            data.extend(out.address.bytes)

            # Amount (8 bytes)
            data.extend(struct.pack("<Q", out.amount))

            # Asset ID (32 bytes)
            data.extend(out.asset_id)

            # Metadata
            meta_len = len(out.metadata) if out.metadata else 0
            data.extend(self._varint_encode(meta_len))
            if meta_len > 0:
                data.extend(out.metadata)

        # LockTime (4 bytes)
        data.extend(struct.pack("<I", self.lock_time))

        return bytes(data)

    @staticmethod
    def deserialize(data: bytes) -> "Transaction":
        """
        Deserialize a transaction from bytes.

        Args:
            data: Serialized transaction

        Returns:
            Transaction: Deserialized transaction
        """
        buf = io.BytesIO(data)
        # Implementation would parse the serialized data
        # For brevity, returning a placeholder
        raise NotImplementedError("Deserialization not yet implemented")

    def get_tx_id(self) -> str:
        """
        Get transaction ID as hex string.

        Returns:
            str: Transaction ID
        """
        return self.hash().hex()

    def get_output_sum(self) -> int:
        """
        Get total output amount.

        Returns:
            int: Sum of all output amounts
        """
        return sum(out.amount for out in self.outputs)

    def get_fee(self, input_sum: int) -> int:
        """
        Calculate transaction fee.

        Args:
            input_sum: Sum of input amounts

        Returns:
            int: Transaction fee
        """
        return input_sum - self.get_output_sum()

    def sign_with(self, wallet: Wallet):
        """
        Sign the transaction with a wallet.

        Args:
            wallet: Wallet to sign with
        """
        tx_hash = self.hash()
        signature = wallet.sign(tx_hash)

        # Add signature and public key to all inputs
        for inp in self.inputs:
            inp.signature = signature
            inp.public_key = wallet.get_public_key()

    def verify(self) -> bool:
        """
        Verify all input signatures.

        Returns:
            bool: True if all signatures are valid
        """
        tx_hash = self.hash()

        for inp in self.inputs:
            if not inp.signature or not inp.public_key:
                return False

            # Create a temporary wallet for verification
            # In production, use proper Ed25519 verification
            temp_wallet = Wallet(inp.public_key, inp.public_key)
            if not temp_wallet.verify(tx_hash, inp.signature):
                return False

        return True

    def __repr__(self) -> str:
        return f"Transaction(id={self.get_tx_id()[:16]}..., inputs={len(self.inputs)}, outputs={len(self.outputs)})"


# Import io for BytesIO
import io


def _varint_encode(num: int) -> bytes:
    """Encode number as varint."""
    buf = bytearray()
    while num > 0x7F:
        buf.append((num & 0x7F) | 0x80)
        num >>= 7
    buf.append(num)
    return bytes(buf)


def _varint_decode(data: bytes) -> tuple:
    """Decode varint."""
    value = 0
    bytes_read = 0
    shift = 0

    for byte in data:
        bytes_read += 1
        value |= (byte & 0x7F) << shift

        if not (byte & 0x80):
            break
        shift += 7

    return value, bytes_read


# Add methods to Transaction class
Transaction._varint_encode = staticmethod(_varint_encode)
Transaction._varint_decode = staticmethod(_varint_decode)


def create_payment(
    from_wallet: Wallet,
    to_address: str,
    amount: int,
    utxos: List[dict]
) -> Transaction:
    """
    Create a simple payment transaction.

    Args:
        from_wallet: Sender's wallet
        to_address: Recipient address (Bech32m)
        amount: Amount to send
        utxos: List of available UTXOs

    Returns:
        Transaction: Signed transaction
    """
    # Build inputs from available UTXOs
    inputs = [TXInput(utxo["tx_hash"], utxo["index"]) for utxo in utxos]

    # Create output
    output = TXOutput(to_address, amount)

    # Create transaction
    tx = Transaction(1, inputs, [output])

    # Sign the transaction
    tx.sign_with(from_wallet)

    return tx


def create_batch_payment(
    from_wallet: Wallet,
    recipients: List[dict],
    utxos: List[dict]
) -> Transaction:
    """
    Create a batch payment transaction to multiple recipients.

    Args:
        from_wallet: Sender's wallet
        recipients: List of { address, amount }
        utxos: List of available UTXOs

    Returns:
        Transaction: Signed transaction
    """
    inputs = [TXInput(utxo["tx_hash"], utxo["index"]) for utxo in utxos]
    outputs = [TXOutput(r["address"], r["amount"]) for r in recipients]

    tx = Transaction(1, inputs, outputs)
    tx.sign_with(from_wallet)

    return tx


# Example usage
if __name__ == "__main__":
    from .wallet import Wallet, Address

    # Create a new wallet
    wallet = Wallet.create()
    print(f"Wallet: {wallet.get_address_string()}")

    # Simulate UTXOs (in real usage, fetch from blockchain)
    utxos = [
        {"tx_hash": "0" * 32, "index": 0},  # Placeholder
    ]

    # Create a payment
    # Note: This is a demonstration, not a real transaction
    tx = Transaction.build(
        [{"tx_hash": "a" * 32, "index": 0}],
        [{"address": wallet.get_address_string(), "amount": 1000}]
    )

    print(f"Transaction: {tx.get_tx_id()}")
    print(f"Serialized: {tx.serialize().hex()[:64]}...")
