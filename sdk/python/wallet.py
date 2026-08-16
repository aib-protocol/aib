"""
AIB 2.0 SDK for Python

AIB 2.0 Specifications:
    - Cryptographic Algorithm: Ed25519
    - Address Format: Bech32m (HRP: "aib")
    - Transaction Model: UTXO

This module provides wallet functionality for the AIB blockchain.
"""

import hashlib
import secrets
from typing import Optional, Union
import base64


class Address:
    """Represents a 32-byte AIB blockchain address."""

    CHARSET = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"

    def __init__(self, bytes_data: bytes):
        if len(bytes_data) != 32:
            raise ValueError("Address must be 32 bytes")
        self.bytes = bytes_data

    def to_hex(self) -> str:
        """Convert address to hex string."""
        return self.bytes.hex()

    def to_bech32(self) -> str:
        """Convert address to Bech32m format."""
        return self._encode_bech32m("aib", self.bytes)

    @staticmethod
    def from_bech32(bech32_string: str) -> "Address":
        """Create address from Bech32m string."""
        bytes_data = Address._decode_bech32m(bech32_string)
        return Address(bytes_data)

    @staticmethod
    def from_hex(hex_string: str) -> "Address":
        """Create address from hex string."""
        return Address(bytes.fromhex(hex_string))

    @staticmethod
    def _encode_bech32m(hrp: str, data: bytes) -> str:
        """
        Encode data to Bech32m format.
        Simplified implementation for demonstration.
        For production, use a proper bech32m library.
        """
        # Compute checksum
        checksum_data = hrp.encode() + data
        checksum = hashlib.sha256(hashlib.sha256(checksum_data).digest()).digest()

        # Take 5-bit chunks
        bits = []
        for byte in data:
            bits.extend([(byte >> i) & 1 for i in range(7, -1, -1)])

        encoded = ""
        for i in range(0, len(bits) - 3, 5):
            index = (bits[i] << 4) | (bits[i + 1] << 3) | (bits[i + 2] << 2) | (bits[i + 3] << 1) | bits[i + 4]
            encoded += Address.CHARSET[index]

        # Add checksum (6 characters)
        for i in range(6):
            encoded += Address.CHARSET[checksum[i] % 32]

        return hrp + "1" + encoded

    @staticmethod
    def _decode_bech32m(address: str) -> bytes:
        """Decode Bech32m address to bytes."""
        if len(address) < 6:
            raise ValueError("Address too short")

        separator_index = address.find("1")
        if separator_index == -1:
            raise ValueError("Invalid address format")

        hrp = address[:separator_index]
        if hrp != "aib":
            raise ValueError(f"Invalid HRP: expected 'aib', got '{hrp}'")

        data_str = address[separator_index + 1:-6]  # Remove checksum

        bits = []
        for c in data_str:
            index = Address.CHARSET.find(c)
            if index == -1:
                raise ValueError(f"Invalid character: {c}")
            bits.extend([(index >> i) & 1 for i in range(4, -1, -1)])

        # Convert bits back to bytes
        data = bytearray()
        for i in range(0, len(bits) - 7, 8):
            byte = 0
            for j in range(8):
                byte = (byte << 1) | bits[i + j]
            data.append(byte)

        return bytes(data[:32])

    def __str__(self) -> str:
        return self.to_bech32()

    def __repr__(self) -> str:
        return f"Address({self.to_bech32()})"


class Wallet:
    """
    Represents a cryptographic wallet with Ed25519 key pair.
    """

    def __init__(self, private_key: bytes, public_key: bytes):
        """
        Initialize wallet with private and public keys.

        Args:
            private_key: 32-byte private key
            public_key: 32-byte public key
        """
        self._private_key = private_key
        self._public_key = public_key
        # Derive address from public key using SHA256
        address_hash = hashlib.sha256(public_key).digest()
        self._address = Address(address_hash)

    @staticmethod
    def create() -> "Wallet":
        """
        Create a new wallet with a randomly generated key pair.

        Returns:
            Wallet: New wallet instance
        """
        # Generate random seed
        seed = secrets.token_bytes(32)
        return Wallet.from_seed(seed)

    @staticmethod
    def from_seed(seed: bytes) -> "Wallet":
        """
        Create a wallet from a 32-byte seed.

        Args:
            seed: 32-byte seed

        Returns:
            Wallet: New wallet instance
        """
        if len(seed) != 32:
            raise ValueError("Seed must be 32 bytes")

        # Use seed as private key (Ed25519 key derivation)
        private_key = seed
        # Derive public key (for Ed25519, public key is derived from private key)
        public_key = Wallet._derive_public_key(private_key)

        return Wallet(private_key, public_key)

    @staticmethod
    def from_private_key(private_key_hex: str) -> "Wallet":
        """
        Import a wallet from an existing private key.

        Args:
            private_key_hex: Private key as hex string

        Returns:
            Wallet: New wallet instance
        """
        private_key_bytes = bytes.fromhex(private_key_hex)

        if len(private_key_bytes) != 32:
            raise ValueError("Private key must be 32 bytes")

        public_key = Wallet._derive_public_key(private_key_bytes)

        return Wallet(private_key_bytes, public_key)

    @staticmethod
    def _derive_public_key(private_key: bytes) -> bytes:
        """
        Derive public key from private key.
        This is a simplified Ed25519 public key derivation.
        In production, use cryptography library.
        """
        # Simplified: for demonstration, use SHA256 of private key
        # Real Ed25519 implementation would use point multiplication
        return hashlib.sha256(private_key).digest()

    def get_address(self) -> Address:
        """Get the wallet's address."""
        return self._address

    def get_address_string(self) -> str:
        """Get the wallet's address as Bech32m string."""
        return self._address.to_bech32()

    def get_public_key(self) -> bytes:
        """Get the wallet's public key."""
        return self._public_key

    def get_public_key_hex(self) -> str:
        """Get the wallet's public key as hex string."""
        return self._public_key.hex()

    def get_private_key(self) -> bytes:
        """
        Get the wallet's private key.

        WARNING: Keep this secure! Anyone with the private key can control the funds.
        """
        return self._private_key

    def get_private_key_hex(self) -> str:
        """
        Get the wallet's private key as hex string.

        WARNING: Keep this secure!
        """
        return self._private_key.hex()

    def sign(self, message: bytes) -> bytes:
        """
        Sign a message with the wallet's private key.
        Simplified implementation for demonstration.
        """
        # Simplified: use HMAC-SHA256 as signature
        # Real Ed25519 implementation would use proper signing
        import hmac
        return hmac.new(self._private_key, message, hashlib.sha512).digest()[:64]

    def verify(self, message: bytes, signature: bytes) -> bool:
        """
        Verify a signature for a message.
        Simplified implementation for demonstration.
        """
        expected_signature = self.sign(message)
        return hmac.compare_digest(expected_signature, signature)

    def __repr__(self) -> str:
        return f"Wallet(address={self.get_address_string()})"


# Example usage and demonstration
if __name__ == "__main__":
    # Create a new wallet
    wallet = Wallet.create()
    print(f"Created wallet: {wallet.get_address_string()}")
    print(f"Public key: {wallet.get_public_key_hex()}")

    # Import existing wallet from private key
    # wallet2 = Wallet.from_private_key("your-private-key-hex")

    # Sign a message
    message = b"Hello, AIB!"
    signature = wallet.sign(message)
    print(f"Signature: {signature.hex()}")

    # Verify signature
    is_valid = wallet.verify(message, signature)
    print(f"Signature valid: {is_valid}")
